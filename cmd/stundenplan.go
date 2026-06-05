package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/stundenplan"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var stundenplanCmd = &cobra.Command{
	Use:     "stundenplan",
	Aliases: []string{"plan", "timetable"},
	Short:   "List and download timetable calendars (Stundenpläne, per Zenturie)",
	Long: `Lists the downloadable timetable files (.ics calendars and .html overviews)
that the CIS publishes per Zenturie.

  cis stundenplan                       # list everything
  cis stundenplan --zenturie I24a       # only your group
  cis stundenplan -z I24a -f ics        # only the ICS calendar
  cis stundenplan -z I24a -f ics -d -o ~/Downloads   # download it`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.New()
		if err != nil {
			return err
		}
		if !c.IsLoggedIn() {
			return fmt.Errorf("not logged in — run 'cis login' first")
		}

		zenturie, _ := cmd.Flags().GetString("zenturie")
		format, _ := cmd.Flags().GetString("format")
		asJSON, _ := cmd.Flags().GetBool("json")
		download, _ := cmd.Flags().GetBool("download")
		outDir, _ := cmd.Flags().GetString("out")

		fmt.Fprintln(os.Stderr, "Fetching Stundenpläne...")
		all, err := stundenplan.FetchList(c)
		if err != nil {
			return err
		}
		plans := stundenplan.Filter(all, zenturie, format)
		if len(plans) == 0 {
			fmt.Println("No timetable files matched.")
			return nil
		}

		if download {
			if len(plans) != 1 {
				return fmt.Errorf("download needs exactly one match, got %d — narrow with --zenturie/--format", len(plans))
			}
			p := plans[0]
			fmt.Fprintf(os.Stderr, "Downloading %s...\n", p.Filename)
			data, _, err := stundenplan.Download(c, p.URL)
			if err != nil {
				return err
			}
			out := p.Filename
			if outDir != "" {
				out = filepath.Join(expandHome(outDir), p.Filename)
			}
			if err := os.WriteFile(out, data, 0600); err != nil {
				return fmt.Errorf("write file: %w", err)
			}
			fmt.Printf("Saved: %s (%d bytes)\n", out, len(data))
			return nil
		}

		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(plans)
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.SetStyle(table.StyleLight)
		t.SetTitle("Stundenpläne")
		t.AppendHeader(table.Row{"Zenturie", "Format", "Stand", "Größe"})
		for _, p := range plans {
			t.AppendRow(table.Row{p.Zenturie, p.Format, p.Date, p.Size})
		}
		t.Render()
		fmt.Fprintln(os.Stderr, "\nUse -z <Zenturie> -f ics -d to download a calendar.")
		return nil
	},
}

// expandHome resolves a leading ~/ to the user's home directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func init() {
	stundenplanCmd.Flags().StringP("zenturie", "z", "", "Filter by Zenturie prefix (e.g. I24a)")
	stundenplanCmd.Flags().StringP("format", "f", "", "Filter by format (ics/html)")
	stundenplanCmd.Flags().Bool("json", false, "Output as JSON")
	stundenplanCmd.Flags().BoolP("download", "d", false, "Download the single matching file")
	stundenplanCmd.Flags().StringP("out", "o", ".", "Output directory for downloads")
	rootCmd.AddCommand(stundenplanCmd)
}
