package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirmByTyping prints a prompt and requires the user to type the expected
// token exactly (case-insensitive, trimmed) before a binding write action runs.
// It returns true only on an exact match. Used as a deliberate second gate in
// front of irreversible CIS submissions.
func confirmByTyping(expected string) bool {
	fmt.Fprintf(os.Stderr, "Type %q to confirm (anything else aborts): ", expected)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(line), strings.TrimSpace(expected))
}
