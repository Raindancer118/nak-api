package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/wahlpflicht"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var wpfCmd = &cobra.Command{
	Use:     "wpf",
	Aliases: []string{"wahlpflicht"},
	Short:   "List Wahlpflichtmodule and select one (binding)",
	Long: `Lists the Wahlpflichtmodule for your curriculum, shows module details and
selects a module. Selecting is BINDING and only runs with --confirm plus an
interactive confirmation.

  cis wpf                       # list modules
  cis wpf --detail 1234         # module detail (+ whether selection is open)
  cis wpf --select 1234         # dry run: shows what would be selected
  cis wpf --select 1234 --confirm     # actually select (asks again)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if !c.IsLoggedIn() {
			return fmt.Errorf("not logged in — run 'cis login' first")
		}

		detailID, _ := cmd.Flags().GetString("detail")
		selectID, _ := cmd.Flags().GetString("select")
		confirm, _ := cmd.Flags().GetBool("confirm")
		asJSON, _ := cmd.Flags().GetBool("json")

		switch {
		case detailID != "":
			d, err := wahlpflicht.FetchDetail(c, detailID)
			if err != nil {
				return err
			}
			if asJSON {
				return encodeJSON(d)
			}
			fmt.Printf("%s\n\n", d.Title)
			fmt.Printf("Credits:   %s\n", d.Credits)
			fmt.Printf("Prüfung:   %s\n", d.Pruefung)
			fmt.Printf("Dozenten:  %v\n", d.Dozenten)
			fmt.Printf("Wählbar:   %v\n", d.SelectAvail)
			if d.Description != "" {
				fmt.Printf("\n%s\n", d.Description)
			}
			return nil

		case selectID != "":
			return runWpfSelect(c, selectID, confirm)

		default:
			fmt.Fprintln(os.Stderr, "Fetching Wahlpflichtmodule...")
			mods, err := wahlpflicht.FetchModules(c)
			if err != nil {
				return err
			}
			if len(mods) == 0 {
				fmt.Println("No Wahlpflichtmodule found.")
				return nil
			}
			if asJSON {
				return encodeJSON(mods)
			}
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.SetStyle(table.StyleLight)
			t.SetTitle("Wahlpflichtmodule")
			t.AppendHeader(table.Row{"ID", "Titel"})
			for _, m := range mods {
				t.AppendRow(table.Row{m.ID, m.Title})
			}
			t.Render()
			fmt.Fprintln(os.Stderr, "\nDetails:  cis wpf --detail <ID>   Select:  cis wpf --select <ID> --confirm")
			return nil
		}
	},
}

func runWpfSelect(c *client.Client, moduleID string, confirm bool) error {
	d, err := wahlpflicht.FetchDetail(c, moduleID)
	if err != nil {
		return err
	}
	fmt.Printf("Modul:    %s (ID %s)\n", d.Title, d.ID)
	fmt.Printf("Wählbar:  %v\n", d.SelectAvail)
	if !d.SelectAvail {
		return fmt.Errorf("selection is not currently open for this module")
	}

	if !confirm {
		fmt.Println("\nDRY RUN — nothing was sent.")
		fmt.Println("Re-run with --confirm to perform the binding selection.")
		return nil
	}

	fmt.Printf("\n⚠  VERBINDLICHE WAHLPFLICHT-WAHL — this cannot be undone automatically.\n")
	if !confirmByTyping(d.ID) {
		fmt.Println("Aborted.")
		return nil
	}
	msg, err := wahlpflicht.Select(c, moduleID)
	if err != nil {
		return err
	}
	fmt.Printf("\n✓ %s\n", msg)
	return nil
}

func encodeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	wpfCmd.Flags().String("detail", "", "Show details for a module ID")
	wpfCmd.Flags().String("select", "", "Select the module with this ID")
	wpfCmd.Flags().Bool("confirm", false, "Actually perform the binding selection (otherwise dry run)")
	wpfCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(wpfCmd)
}
