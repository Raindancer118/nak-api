// Package cmd is the nak command line: the MCP server plus a generic way to
// run every tool from a shell.
package cmd

import (
	"fmt"
	"github.com/Raindancer118/nak-api/internal/tools"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/Raindancer118/nak-api/cmd.Version=…".
var Version = "0.1.0-dev"

var rootCmd = &cobra.Command{
	Use:     "nak",
	Short:   "NORDAKADEMIE API: CIS + Moodle as one MCP server and CLI",
	Version: Version,
	Long: `nak bundles the NORDAKADEMIE Campus Information System (CIS) and Moodle.

  nak mcp                       start the MCP server (stdio)
  nak tools                     list all tools
  nak tool <name> key=value …   run one tool, JSON output

Credentials: CIS_USER/CIS_PASS (Moodle falls back to them; MOODLE_USER/MOODLE_PASS
override). NAK_READONLY=1 blocks every write in both systems.`,
	SilenceUsage: true,
}

func Execute() {
	rootCmd.Version = Version
	tools.Version = Version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
