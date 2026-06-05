package stundenplan

import (
	"os"
	"testing"
)

func loadFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/stundenplan.html")
	if err != nil {
		// Fixtures hold personal CIS data and are git-ignored; skip when absent.
		t.Skipf("fixture not present (kept local for privacy): %v", err)
	}
	return string(b)
}

func TestParsePlans(t *testing.T) {
	plans := parsePlans(loadFixture(t))
	if len(plans) == 0 {
		t.Fatal("expected plans, got none")
	}

	// Every plan must have the essential fields populated.
	for _, p := range plans {
		if p.Filename == "" || p.URL == "" || p.Zenturie == "" {
			t.Errorf("incomplete plan: %+v", p)
		}
		if p.Format == "" {
			t.Errorf("missing format for %s", p.Filename)
		}
	}

	// The download URL must be absolute and have entities decoded.
	for _, p := range plans {
		if got := p.URL; len(got) < 4 || got[:4] != "http" {
			t.Errorf("url not absolute: %q", got)
		}
		if contains(p.URL, "&amp;") {
			t.Errorf("url still HTML-encoded: %q", p.URL)
		}
	}
}

func TestParsePlansKnownEntry(t *testing.T) {
	plans := parsePlans(loadFixture(t))

	// The fixture is known to contain an HTML schedule for the A24a_4 group.
	var found *Plan
	for i := range plans {
		if plans[i].Filename == "A24a_4.html" {
			found = &plans[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected A24a_4.html in fixture")
	}
	if found.Zenturie != "A24a_4" {
		t.Errorf("zenturie = %q, want A24a_4", found.Zenturie)
	}
	if found.Format != "html" {
		t.Errorf("format = %q, want html", found.Format)
	}
	if found.Date != "13.05.2026" {
		t.Errorf("date = %q, want 13.05.2026", found.Date)
	}
	if found.Size != "126 KB" {
		t.Errorf("size = %q, want 126 KB", found.Size)
	}
}

func TestFilter(t *testing.T) {
	plans := parsePlans(loadFixture(t))

	// Filter by Zenturie prefix is case-insensitive.
	icsW := Filter(plans, "w24", "ics")
	if len(icsW) == 0 {
		t.Fatal("expected W24* ICS plans")
	}
	for _, p := range icsW {
		if p.Format != "ics" {
			t.Errorf("format filter leaked: %+v", p)
		}
		if len(p.Zenturie) < 3 || toLower(p.Zenturie[:3]) != "w24" {
			t.Errorf("zenturie filter leaked: %+v", p)
		}
	}

	// Empty filter is a passthrough.
	if len(Filter(plans, "", "")) != len(plans) {
		t.Error("empty filter must return all plans")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
