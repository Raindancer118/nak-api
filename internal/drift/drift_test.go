package drift

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestErrorIsDrift(t *testing.T) {
	err := fmt.Errorf("wrap: %w", New("/x", "grade table (#curricular)", "<main/>"))
	if !errors.Is(err, ErrDrift) {
		t.Fatal("not recognised as drift")
	}
	var d *Error
	if !errors.As(err, &d) || d.Page != "/x" || d.Expected != "grade table (#curricular)" {
		t.Fatalf("details lost: %+v", d)
	}
}

// The fingerprint goes into PUBLIC issues: structure only, never content.
func TestFingerprintHasStructureButNoPersonalData(t *testing.T) {
	b, err := os.ReadFile("../grades/testdata/leistungsuebersicht.html")
	if err != nil {
		t.Fatal(err)
	}
	fp := Fingerprint(string(b))
	s := fp.String()
	for _, want := range []string{"Modulnummer", "Note:", "curricular", "tx_nagrades_nagradesmodules"} {
		if !strings.Contains(s, want) {
			t.Errorf("fingerprint misses %q:\n%s", want, s)
		}
	}
	for _, leak := range []string{"Mustermann", "Automatentheorie", "1,0", "22.04.2025", "cHash", "performanceId%5D=2"} {
		if strings.Contains(s, leak) {
			t.Errorf("fingerprint leaks %q:\n%s", leak, s)
		}
	}
	if len(s) > 4000 {
		t.Errorf("fingerprint too long: %d", len(s))
	}
}
