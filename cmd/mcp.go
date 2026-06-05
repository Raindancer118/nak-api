package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Raindancer118/cis-api/internal/auth"
	"github.com/Raindancer118/cis-api/internal/certs"
	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/exams"
	"github.com/Raindancer118/cis-api/internal/grades"
	"github.com/Raindancer118/cis-api/internal/seminars"
	"github.com/Raindancer118/cis-api/internal/stundenplan"
	"github.com/Raindancer118/cis-api/internal/transfer"
	"github.com/Raindancer118/cis-api/internal/wahlpflicht"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (stdio transport) for use with Claude Code",
	Long: `Starts an MCP server over stdio. Add to Claude Code settings:

  {
    "mcpServers": {
      "cis": {
        "command": "/path/to/cis",
        "args": ["mcp"]
      }
    }
  }

Set CIS_USER and CIS_PASS environment variables, or run 'cis login' first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}

		// Auto-login if env vars are set and no saved session
		if !c.IsLoggedIn() {
			u := os.Getenv("CIS_USER")
			p := os.Getenv("CIS_PASS")
			if u != "" && p != "" {
				if err := auth.Login(c, u, p); err != nil {
					return fmt.Errorf("auto-login failed: %w", err)
				}
			}
		}

		s := server.NewMCPServer(
			"cis-api",
			"1.0.0",
			server.WithToolCapabilities(true),
		)

		registerTools(s, c)

		return server.ServeStdio(s)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func registerTools(s *server.MCPServer, c *client.Client) {
	// ── Auth ──────────────────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_login",
			mcp.WithDescription("Log in to the NORDAKADEMIE CIS portal. Session is saved locally."),
			mcp.WithString("username", mcp.Required(), mcp.Description("Your CIS username (student ID)")),
			mcp.WithString("password", mcp.Required(), mcp.Description("Your CIS password")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			u, _ := req.RequireString("username")
			p, _ := req.RequireString("password")
			if err := auth.Login(c, u, p); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("Logged in successfully. Session saved."), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_logout",
			mcp.WithDescription("Log out from the CIS portal and remove the local session."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := auth.Logout(c); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("Logged out."), nil
		},
	)

	// ── Grades ────────────────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_grades",
			mcp.WithDescription("Fetch your grades (Leistungsübersicht / Notenspiegel) from the CIS."),
			mcp.WithString("lang", mcp.Description("Language for transcript: 'de' (default) or 'en'")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			lang := req.GetString("lang", "de")
			gs, _, err := grades.FetchAll(c, lang)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(gs)), nil
		},
	)

	// ── Stundenplan ───────────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_list_stundenplan",
			mcp.WithDescription("List downloadable timetable files (Stundenpläne) per Zenturie. Each Zenturie has an .ics calendar and an .html overview."),
			mcp.WithString("zenturie", mcp.Description("Optional Zenturie prefix filter, e.g. 'I24a'")),
			mcp.WithString("format", mcp.Description("Optional format filter: 'ics' or 'html'")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			all, err := stundenplan.FetchList(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			plans := stundenplan.Filter(all, req.GetString("zenturie", ""), req.GetString("format", ""))
			return mcp.NewToolResultText(toJSON(plans)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_download_stundenplan",
			mcp.WithDescription("Download a timetable file (e.g. an .ics calendar) to a local path. Get the URL from cis_list_stundenplan."),
			mcp.WithString("download_url", mcp.Required(), mcp.Description("Download URL (from cis_list_stundenplan)")),
			mcp.WithString("output_path", mcp.Required(), mcp.Description("Local file path, e.g. ~/Downloads/I24a.ics")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			dlURL, _ := req.RequireString("download_url")
			outPath, _ := req.RequireString("output_path")
			outPath = expandHome(outPath)
			data, _, err := stundenplan.Download(c, dlURL)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := os.WriteFile(outPath, data, 0600); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("write file: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Saved %d bytes to %s", len(data), outPath)), nil
		},
	)

	// ── Klausuren / Prüfungen ─────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_list_klausuren",
			mcp.WithDescription("List the exam overview (Prüfungsübersicht) with each exam's examId, dates and whether registration/deregistration is currently offered."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			list, err := exams.FetchList(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(list)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_klausur_action",
			mcp.WithDescription("Register for or deregister from an exam. THIS IS A BINDING WRITE ACTION. Without confirm=true it only returns a dry-run preview and submits nothing. Always show the preview to the user and get explicit approval before calling again with confirm=true."),
			mcp.WithString("exam_id", mcp.Required(), mcp.Description("ExamID (from cis_list_klausuren)")),
			mcp.WithString("action", mcp.Required(), mcp.Description("'register' or 'deregister' — must match what the page currently offers for this exam")),
			mcp.WithBoolean("confirm", mcp.Description("Set true to actually submit the binding request. Default false = dry run.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			examID, _ := req.RequireString("exam_id")
			action, _ := req.RequireString("action")
			if action != "register" && action != "deregister" {
				return mcp.NewToolResultError("action must be 'register' or 'deregister'"), nil
			}
			e, err := exams.Resolve(c, examID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if e.Action != action {
				return mcp.NewToolResultError(fmt.Sprintf("exam %s currently offers %q, not %q", examID, e.Action, action)), nil
			}
			preview := fmt.Sprintf("Prüfung: %s %s | ExamID %s | %s–%s | action=%s",
				e.ModuleNr, e.Title, e.ExamID, e.Start, e.Ende, e.Action)
			if !req.GetBool("confirm", false) {
				return mcp.NewToolResultText("DRY RUN — nothing submitted.\n" + preview +
					"\nCall again with confirm=true to perform this BINDING action."), nil
			}
			msg, err := exams.Submit(c, e.ActionURL)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("SUBMITTED: " + preview + "\nResult: " + msg), nil
		},
	)

	// ── Seminars ──────────────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_grades_debug",
			mcp.WithDescription("Debug tool: shows all tables found on the Leistungsübersicht page, their headers, and transcript links. Use when cis_grades returns null."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			info, err := grades.FetchDebugInfo(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(info)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_list_seminars",
			mcp.WithDescription("List all available seminars. Shows title, ID, and whether a waitlist is open."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			list, err := seminars.FetchList(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(list)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_seminar_detail",
			mcp.WithDescription("Get details of a specific seminar (description, dozent, dates, credits)."),
			mcp.WithString("seminar_id", mcp.Required(), mcp.Description("Seminar ID (from cis_list_seminars)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			id, _ := req.RequireString("seminar_id")
			detail, err := seminars.FetchDetail(c, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(detail)), nil
		},
	)

	// ── Transferleistungen ────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_list_transfer",
			mcp.WithDescription("List Transferleistungen / Praxisberichte (overview) with their id, Thema, Modul, Wertung and Status."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			list, err := transfer.FetchList(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(list)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_transfer_bewertung",
			mcp.WithDescription("Grading detail of one Transferleistung: per-criterion notes, weights, feedback, plus a client-side weighted Gesamtnote (the CIS itself publishes no overall grade)."),
			mcp.WithString("transfer_id", mcp.Required(), mcp.Description("transferTermPaperId (from cis_list_transfer)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			id, _ := req.RequireString("transfer_id")
			b, err := transfer.FetchBewertung(c, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(b)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_download_transfer_document",
			mcp.WithDescription("Download a Transferleistung attachment (e.g. the report PDF) to a local path. URLs come from cis_transfer_bewertung."),
			mcp.WithString("download_url", mcp.Required(), mcp.Description("Document URL (from cis_transfer_bewertung)")),
			mcp.WithString("output_path", mcp.Required(), mcp.Description("Local file path, e.g. ~/Downloads/transfer.pdf")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			dlURL, _ := req.RequireString("download_url")
			outPath, _ := req.RequireString("output_path")
			outPath = expandHome(outPath)
			data, _, err := transfer.Download(c, dlURL)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := os.WriteFile(outPath, data, 0600); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("write file: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Saved %d bytes to %s", len(data), outPath)), nil
		},
	)

	// ── Wahlpflichtmodule ─────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_list_wahlpflicht",
			mcp.WithDescription("List available Wahlpflicht modules (mandatory elective courses) for your curriculum."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			list, err := wahlpflicht.FetchModules(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(list)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_wahlpflicht_detail",
			mcp.WithDescription("Get details of a Wahlpflicht module. Also shows whether selection is currently available."),
			mcp.WithString("module_id", mcp.Required(), mcp.Description("Module ID (from cis_list_wahlpflicht)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			id, _ := req.RequireString("module_id")
			detail, err := wahlpflicht.FetchDetail(c, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(detail)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_select_wahlpflicht",
			mcp.WithDescription("Select a Wahlpflicht module. Only works when the selection period is open."),
			mcp.WithString("module_id", mcp.Required(), mcp.Description("Module ID to select (from cis_list_wahlpflicht)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			id, _ := req.RequireString("module_id")
			msg, err := wahlpflicht.Select(c, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(msg), nil
		},
	)

	// ── Certificates ──────────────────────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("cis_list_certs",
			mcp.WithDescription("List downloadable certificates (Immatrikulationsbescheinigungen etc.)."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			list, err := certs.FetchList(c)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(list)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("cis_download_cert",
			mcp.WithDescription("Download a certificate to a local file."),
			mcp.WithString("download_url", mcp.Required(), mcp.Description("Download URL (from cis_list_certs)")),
			mcp.WithString("output_path", mcp.Required(), mcp.Description("Local file path to save the certificate (e.g. ~/Downloads/immatrikulation.pdf)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !c.IsLoggedIn() {
				return mcp.NewToolResultError("not logged in — call cis_login first"), nil
			}
			dlURL, _ := req.RequireString("download_url")
			outPath, _ := req.RequireString("output_path")
			outPath = expandHome(outPath)
			data, _, err := certs.Download(c, dlURL)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := os.WriteFile(outPath, data, 0600); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("write file: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Saved %d bytes to %s", len(data), outPath)), nil
		},
	)
}

func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "%v"}`, err)
	}
	return string(b)
}
