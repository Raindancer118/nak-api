package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cis",
	Short: "CLI client for the NORDAKADEMIE Campus Information System",
	Long:  "cis interacts with cis.nordakademie.de using a cookie-based session.\nRun 'cis login' first, then use 'cis grades' or 'cis certs'.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(gradesCmd)
	rootCmd.AddCommand(certsCmd)
}
