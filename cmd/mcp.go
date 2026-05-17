package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Raindancer118/cis-api/internal/auth"
	"github.com/Raindancer118/cis-api/internal/certs"
	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/grades"
	"github.com/Raindancer118/cis-api/internal/seminars"
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

	// ── Seminars ──────────────────────────────────────────────────────────────

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
			if strings.HasPrefix(outPath, "~/") {
				home, _ := os.UserHomeDir()
				outPath = home + outPath[1:]
			}
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
