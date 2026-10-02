package certs

import (
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	b, err := os.ReadFile("testdata/bescheinigungen.html")
	if err != nil {
		t.Fatal(err)
	}
	list := Parse(string(b), "https://cis.example")
	if len(list) != 14 {
		t.Fatalf("certs = %d", len(list))
	}
	c := list[0]
	if c.Semester != "WS 2023" || c.Period != "01.10.2023 - 31.03.2024" || c.Lang != "de" || c.Name != "Studienbescheinigung WS 2023 (de)" {
		t.Errorf("first = %+v", c)
	}
	if list[1].Lang != "en" {
		t.Errorf("second lang = %q", list[1].Lang)
	}
	newest, err := Find(list, "", "en")
	if err != nil || newest.Semester != "WS 2026" || newest.Lang != "en" {
		t.Errorf("newest = %+v %v", newest, err)
	}
	ss, err := Find(list, "ss 2025", "")
	if err != nil || ss.Semester != "SS 2025" || ss.Lang != "de" {
		t.Errorf("ss = %+v %v", ss, err)
	}
	if _, err := Find(list, "WS 1999", "de"); err == nil {
		t.Error("unknown semester found")
	}
}
