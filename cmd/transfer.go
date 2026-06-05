package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/transfer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

var transferCmd = &cobra.Command{
	Use:     "transfer",
	Aliases: []string{"tl", "transferleistung"},
	Short:   "List Transferleistungen and view their grading (Bewertung)",
	Long: `Lists your Transferleistungen / Praxisberichte and shows the grading detail
of a single report, including a client-side weighted Gesamtnote.

  cis transfer                       # overview of all reports
  cis transfer --bewertung 14534     # grading detail + computed Gesamtnote
  cis transfer -b 14534 --json       # same, as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if !c.IsLoggedIn() {
			return fmt.Errorf("not logged in — run 'cis login' first")
		}

		bewertungID, _ := cmd.Flags().GetString("bewertung")
		asJSON, _ := cmd.Flags().GetBool("json")

		if bewertungID != "" {
			b, err := transfer.FetchBewertung(c, bewertungID)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(b)
			}
			printBewertung(b)
			return nil
		}

		fmt.Fprintln(os.Stderr, "Fetching Transferleistungen...")
		reports, err := transfer.FetchList(c)
		if err != nil {
			return err
		}
		if len(reports) == 0 {
			fmt.Println("No Transferleistungen found.")
			return nil
		}
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(reports)
		}
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetStyle(table.StyleLight)
		t.SetTitle("Transferleistungen")
		t.AppendHeader(table.Row{"ID", "Nr", "Thema", "Modul", "Wertung", "Status"})
		for _, r := range reports {
			t.AppendRow(table.Row{r.ID, r.No, truncate(r.Topic, 40), truncate(r.Module, 30), r.Wertung, r.Status})
		}
		t.Render()
		fmt.Fprintln(os.Stderr, "\nGrading detail:  cis transfer --bewertung <ID>")
		return nil
	},
}

func printBewertung(b *transfer.Bewertung) {
	for _, key := range []string{"Matrikel-Nr.", "Bericht Nr.", "Versuch", "Thema", "Modul", "Sprache"} {
		if v, ok := b.Fields[key]; ok {
			fmt.Printf("%-14s %s\n", key+":", v)
		}
	}
	fmt.Println()

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.SetTitle("Bewertungskriterien")
	t.AppendHeader(table.Row{"Kriterium", "Note", "Gew.", "Feedback"})
	for _, k := range b.Kriterien {
		t.AppendRow(table.Row{truncate(k.Kriterium, 45), k.Note, k.Gewichtung, truncate(k.Feedback, 45)})
	}
	if b.HasGesamtnote {
		t.AppendFooter(table.Row{"Berechnete Gesamtnote (gewichtet)", fmt.Sprintf("%.2f", b.Gesamtnote), "", ""})
	}
	t.Style().Format.Footer = text.FormatDefault
	t.Render()
	if b.HasGesamtnote {
		fmt.Fprintln(os.Stderr, "\nHinweis: Das CIS weist keine offizielle Gesamtnote aus — dieser Wert ist clientseitig gewichtet berechnet.")
	}
	if len(b.Documents) > 0 {
		fmt.Println("\nDokumente:")
		for _, d := range b.Documents {
			fmt.Printf("  %-20s %s\n", d.Label, d.URL)
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func init() {
	transferCmd.Flags().StringP("bewertung", "b", "", "Show grading detail for a report (transferTermPaperId)")
	transferCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(transferCmd)
}
