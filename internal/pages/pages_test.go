package pages

import (
	"os"
	"strings"
	"testing"
)

func TestParsePruefungsplan(t *testing.T) {
	b, err := os.ReadFile("testdata/pruefungsplan.html")
	if err != nil {
		t.Fatal(err)
	}
	p := Parse(string(b), "https://cis.nordakademie.de")
	if p.Title != "Prüfungsplan" || !strings.Contains(p.Text, "Prüfungsverlaufsplan") {
		t.Errorf("title=%q text=%.80q", p.Title, p.Text)
	}
	if len(p.Downloads) != 15 {
		t.Fatalf("downloads = %d", len(p.Downloads))
	}
	d := p.Downloads[3]
	if d.Text != "B.Sc. Wirtschaftsinformatik" || !strings.Contains(d.Section, "Jahrgang 2023") || !strings.Contains(d.URL, "eID=dumpFile") {
		t.Errorf("download = %+v", d)
	}
}

func TestAllowed(t *testing.T) {
	ok := []string{"/studium/pruefungen", "/service/ansprechpartner?tx_nateam_nateam%5Baction%5D=show&tx_nateam_nateam%5Bid%5D=1&cHash=x"}
	bad := []string{"/login/?logintype=logout", "/x?tx_naexams_naexams%5Baction%5D=register&cHash=1", "/x?foo=1"}
	for _, u := range ok {
		if err := Allowed(u); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	for _, u := range bad {
		if Allowed(u) == nil {
			t.Errorf("%s allowed", u)
		}
	}
}
