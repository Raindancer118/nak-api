package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/health"
	"github.com/spf13/cobra"
)

var selfcheckCmd = &cobra.Command{
	Use:   "selfcheck",
	Short: "Check (read-only) whether the CIS still looks the way nak expects; --report files GitHub issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := app.FromEnv()
		a.ReadOnly = true // the check never writes to the CIS
		c, err := a.CIS()
		if err != nil {
			return err
		}
		rep := health.Run(c, Version, time.Now())
		if j, _ := cmd.Flags().GetBool("json"); j {
			b, _ := json.MarshalIndent(rep, "", "  ")
			fmt.Println(string(b))
		} else {
			for _, r := range rep.Results {
				line := fmt.Sprintf("%-12s %s", r.Status, r.Check)
				if r.Status != health.StatusOK {
					line += "  — " + r.Detail
				}
				fmt.Println(line)
			}
		}
		if rp, _ := cmd.Flags().GetBool("report"); rp {
			tok := health.Token()
			if tok == "" {
				for _, r := range rep.Drifted() {
					fmt.Fprintln(os.Stderr, "report manually:", health.IssueURL(health.DefaultRepo, r, Version, rep.At))
				}
				return fmt.Errorf("no GitHub token (NAK_GITHUB_TOKEN or 'gh auth login')")
			}
			acts, err := (&health.GitHub{API: "https://api.github.com", Repo: health.DefaultRepo, Token: tok}).Sync(rep)
			for _, x := range acts {
				fmt.Printf("issue %s: %s %s\n", x.Action, x.Check, x.URL)
			}
			if err != nil {
				return err
			}
		}
		if len(rep.Drifted()) > 0 {
			os.Exit(2)
		}
		return nil
	},
}

func init() {
	selfcheckCmd.Flags().Bool("report", false, "open/update/close GitHub issues for drift (label cis-drift)")
	selfcheckCmd.Flags().Bool("json", false, "JSON output")
	rootCmd.AddCommand(selfcheckCmd)
}
