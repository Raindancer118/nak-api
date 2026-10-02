package cmd

import (
	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/tools"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

const instructions = `NORDAKADEMIE: CIS (Campus Information System) und Moodle in einem Server.
Präfixe: cis_* (Noten, Prüfungen, Stundenplan, Seminare, WPF, Transferleistungen, Profil, Bescheinigungen),
moodle_* (Kurse, Materialien, Abgaben, Foren, Nachrichten, Tests), nak_* (systemübergreifend).
Einstieg: nak_dashboard. Fristen aus beiden Systemen: nak_deadlines. Kalender: nak_agenda. Ein Modul komplett: nak_module.
Alle Zeiten Europe/Berlin.

SCHREIBENDE TOOLS (mit ⚠ markiert: Prüfungs-/Seminaranmeldung, WPF-Wahl, Transferleistung beantragen,
Profiländerungen, Moodle-Abgaben/Forum/Nachrichten/Abstimmungen) sind verbindlich. Ohne confirm=true liefern sie
nur eine Vorschau und senden nichts. Zeige dem Nutzer die Vorschau und rufe erst nach seiner ausdrücklichen
Zustimmung mit confirm=true erneut auf. Niemals eigenmächtig bestätigen.
Personenbezogene Daten Dritter (Kommiliton:innen, Teilnehmerlisten) werden nur minimal ausgegeben.`

func newServer(a *app.App) *server.MCPServer {
	s := server.NewMCPServer("nak", Version, server.WithToolCapabilities(false), server.WithInstructions(instructions), server.WithRecovery())
	tools.All().Register(s, a)
	return s
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (stdio transport)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.ServeStdio(newServer(app.FromEnv()))
	},
}

func init() { rootCmd.AddCommand(mcpCmd) }
