package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/tools"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var kindName = map[tools.Kind]string{tools.Read: "read", tools.Local: "local", tools.Write: "WRITE"}

var toolsCmd = &cobra.Command{
	Use:   "tools [filter]",
	Short: "List all tools (WRITE = binding, confirm-gated)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := tools.All()
		for _, t := range reg.All() {
			if len(args) == 1 && !strings.Contains(t.Name, args[0]) {
				continue
			}
			fmt.Printf("%-34s %-5s %s\n", t.Name, kindName[t.Kind], firstSentence(t.Desc))
			if v, _ := cmd.Flags().GetBool("params"); v {
				for _, p := range t.Params {
					req := ""
					if p.Required {
						req = " (required)"
					}
					fmt.Printf("    %s=%s%s  %s\n", p.Name, orDefault(p.Type, "string"), req, p.Desc)
				}
			}
		}
		return nil
	},
}

var toolCmd = &cobra.Command{
	Use:   "tool <name> [key=value ...]",
	Short: "Run one tool; values may be JSON (numbers, true/false, arrays, objects)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := tools.All()
		t := reg.Get(args[0])
		if t == nil {
			return fmt.Errorf("unknown tool %q — see 'nak tools'", args[0])
		}
		in, err := parseArgs(args[1:])
		if err != nil {
			return err
		}
		if t.Kind == tools.Write && truthy(in["confirm"]) {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("confirm=true needs an interactive terminal (typed confirmation)")
			}
			preview, err := reg.Call(app.FromEnv(), t.Name, withoutConfirm(in))
			if err != nil {
				return err
			}
			printJSON(preview)
			fmt.Fprint(os.Stderr, "\n⚠  VERBINDLICHE AKTION. Zum Ausführen exakt JA eintippen: ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if strings.TrimSpace(line) != "JA" {
				return fmt.Errorf("abgebrochen — nichts gesendet")
			}
		}
		res, err := reg.Call(app.FromEnv(), t.Name, in)
		if err != nil {
			return err
		}
		printJSON(res)
		return nil
	},
}

func parseArgs(kvs []string) (map[string]any, error) {
	out := map[string]any{}
	for _, kv := range kvs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("argument %q must be key=value", kv)
		}
		var j any
		if err := json.Unmarshal([]byte(v), &j); err == nil {
			if _, isStr := j.(string); !isStr {
				out[k] = j
				continue
			}
		}
		out[k] = v
	}
	return out, nil
}

func withoutConfirm(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if k != "confirm" {
			out[k] = v
		}
	}
	return out
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	}
	return false
}

func printJSON(v any) {
	if s, ok := v.(string); ok {
		fmt.Println(s)
		return
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(b))
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 && i < 110 {
		return s[:i+1]
	}
	if len([]rune(s)) > 110 {
		return string([]rune(s)[:107]) + "…"
	}
	return s
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// shortcuts for the most common session commands
var loginCmd = &cobra.Command{Use: "login", Short: "Log in to the CIS (CIS_USER/CIS_PASS or interactive)", RunE: func(cmd *cobra.Command, args []string) error {
	in := map[string]any{}
	if os.Getenv("CIS_USER") == "" || os.Getenv("CIS_PASS") == "" {
		r := bufio.NewReader(os.Stdin)
		fmt.Fprint(os.Stderr, "CIS username: ")
		u, _ := r.ReadString('\n')
		fmt.Fprint(os.Stderr, "Password: ")
		p, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		in["username"], in["password"] = strings.TrimSpace(u), string(p)
	}
	res, err := tools.All().Call(app.FromEnv(), "cis_login", in)
	if err != nil {
		return err
	}
	printJSON(res)
	return nil
}}

var logoutCmd = &cobra.Command{Use: "logout", Short: "Log out from the CIS", RunE: func(cmd *cobra.Command, args []string) error {
	res, err := tools.All().Call(app.FromEnv(), "cis_logout", nil)
	if err != nil {
		return err
	}
	printJSON(res)
	return nil
}}

func init() {
	toolsCmd.Flags().Bool("params", false, "also list parameters")
	rootCmd.AddCommand(toolsCmd, toolCmd, loginCmd, logoutCmd)
}
