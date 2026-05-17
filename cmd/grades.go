package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/grades"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

var gradesCmd = &cobra.Command{
	Use:   "grades",
	Short: "Fetch and display your grades (Leistungsübersicht)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if !c.IsLoggedIn() {
			return fmt.Errorf("not logged in — run 'cis login' first")
		}

		lang, _ := cmd.Flags().GetString("lang")
		asJSON, _ := cmd.Flags().GetBool("json")
		listLinks, _ := cmd.Flags().GetBool("links")

		if listLinks {
			links, err := grades.FetchOverview(c)
			if err != nil {
				return err
			}
			fmt.Println("Available transcript links:")
			for i, l := range links {
				fmt.Printf("  [%d] %s\n", i+1, l)
			}
			return nil
		}

		fmt.Fprintf(os.Stderr, "Fetching grades (lang=%s)...\n", lang)
		gs, _, err := grades.FetchAll(c, lang)
		if err != nil {
			return err
		}
		if len(gs) == 0 {
			fmt.Println("No grades found. The page layout may have changed.")
			return nil
		}

		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(gs)
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetStyle(table.StyleLight)
		t.Style().Title.Align = text.AlignCenter
		t.SetTitle("Leistungsübersicht")
		t.AppendHeader(table.Row{"Nr.", "Bezeichnung", "Note", "Credits", "Prüfungsdatum", "Semester", "Status"})
		for _, g := range gs {
			t.AppendRow(table.Row{g.ModuleNumber, g.Module, g.Grade, g.Credits, g.ExamDate, g.Semester, g.Status})
		}
		t.Render()
		return nil
	},
}

func init() {
	gradesCmd.Flags().StringP("lang", "l", "de", "Language for transcript (de/en)")
	gradesCmd.Flags().Bool("json", false, "Output as JSON")
	gradesCmd.Flags().Bool("links", false, "List available transcript links (for debugging)")
}
