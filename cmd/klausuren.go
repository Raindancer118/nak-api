package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/exams"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var klausurenCmd = &cobra.Command{
	Use:     "klausuren",
	Aliases: []string{"exams", "pruefungen"},
	Short:   "List exams and register/deregister (Prüfungsan-/abmeldung)",
	Long: `Lists the exam overview (Prüfungsübersicht) and lets you register or
deregister. Writing actions are BINDING — they default to a dry run and only
execute with --confirm plus an interactive confirmation.

  cis klausuren                          # list exams + their examId
  cis klausuren --register 12022         # dry run: shows what would happen
  cis klausuren --register 12022 --confirm     # actually register (asks again)
  cis klausuren --deregister 12022 --confirm   # actually deregister`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if !c.IsLoggedIn() {
			return fmt.Errorf("not logged in — run 'cis login' first")
		}

		registerID, _ := cmd.Flags().GetString("register")
		deregisterID, _ := cmd.Flags().GetString("deregister")
		confirm, _ := cmd.Flags().GetBool("confirm")
		asJSON, _ := cmd.Flags().GetBool("json")

		if registerID != "" && deregisterID != "" {
			return fmt.Errorf("use either --register or --deregister, not both")
		}
		if registerID != "" {
			return runExamAction(c, registerID, "register", "Anmeldung", confirm)
		}
		if deregisterID != "" {
			return runExamAction(c, deregisterID, "deregister", "Abmeldung", confirm)
		}

		// Default: list.
		fmt.Fprintln(os.Stderr, "Fetching Prüfungsübersicht...")
		list, err := exams.FetchList(c)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println("No exams listed (no open registration window?).")
			return nil
		}
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(list)
		}
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetStyle(table.StyleLight)
		t.SetTitle("Prüfungsübersicht")
		t.AppendHeader(table.Row{"ExamID", "Modul", "Bezeichnung", "Zenturien", "Start", "Status", "Aktion"})
		for _, e := range list {
			action := e.ActionLabel
			if action == "" {
				action = "—"
			}
			t.AppendRow(table.Row{e.ExamID, e.ModuleNr, e.Title, strings.Join(e.Zenturien, ", "), e.Start, e.Status, action})
		}
		t.Render()
		fmt.Fprintln(os.Stderr, "\nRegister with:  cis klausuren --register <ExamID> --confirm")
		return nil
	},
}

// runExamAction resolves the exam from the live page and either prints a dry-run
// preview (default) or, with confirm=true and a typed confirmation, submits the
// binding register/deregister request.
func runExamAction(c *client.Client, examID, wantAction, human string, confirm bool) error {
	fmt.Fprintln(os.Stderr, "Resolving exam...")
	e, err := exams.Resolve(c, examID)
	if err != nil {
		return err
	}
	if e.Action != wantAction {
		return fmt.Errorf("exam %s currently offers %q, not %q — refusing to act",
			examID, e.Action, wantAction)
	}

	fmt.Printf("Prüfung:  %s %s\n", e.ModuleNr, e.Title)
	fmt.Printf("ExamID:   %s\n", e.ExamID)
	fmt.Printf("Termin:   %s – %s\n", e.Start, e.Ende)
	fmt.Printf("Aktion:   %s (%s)\n", human, e.Action)

	if !confirm {
		fmt.Println("\nDRY RUN — nothing was sent.")
		fmt.Printf("Re-run with --confirm to perform the binding %s.\n", human)
		return nil
	}

	fmt.Printf("\n⚠  VERBINDLICHE %s — this cannot be undone automatically.\n", strings.ToUpper(human))
	if !confirmByTyping(e.ModuleNr) {
		fmt.Println("Aborted.")
		return nil
	}
	msg, err := exams.Submit(c, e.ActionURL)
	if err != nil {
		return err
	}
	fmt.Printf("\n✓ %s\n", msg)
	return nil
}

func init() {
	klausurenCmd.Flags().String("register", "", "Register for the exam with this ExamID")
	klausurenCmd.Flags().String("deregister", "", "Deregister from the exam with this ExamID")
	klausurenCmd.Flags().Bool("confirm", false, "Actually perform the binding action (otherwise dry run)")
	klausurenCmd.Flags().Bool("json", false, "Output the exam list as JSON")
	rootCmd.AddCommand(klausurenCmd)
}
