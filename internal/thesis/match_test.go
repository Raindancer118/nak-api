package thesis

import (
	"strings"
	"testing"
)

var people = []Reviewer{
	{Name: "Brandt, Jonas", Department: "Informatik", Areas: []string{"Datenbanksysteme", "Data Engineering", "Softwarearchitektur"}, Load: 0},
	{Name: "Varga, Ilse", Department: "Informatik", Areas: []string{"Rechnernetze", "Cloud Computing", "DevOps"}, Load: 2},
	{Name: "Okafor, Mira", Department: "Informatik", Areas: []string{"IT-Sicherheit", "Kryptographie"}, Load: 0},
	{Name: "Weiss, Hanna", Department: "Wirtschaftswissenschaften", Areas: []string{"Marketing", "Digital Marketing", "Marketing Analytics"}, Load: 0},
	{Name: "Engel, Paul", Department: "Wirtschaftswissenschaften", Areas: []string{"Controlling", "Kosten- und Leistungsrechnung"}, Load: 1},
	{Name: "Kurz, Lea", Department: "Informatik", Areas: []string{"Machine Learning", "Generative KI", "Data Science"}, Load: 0},
}

func top(rs []Match) string { return rs[0].Reviewer.Name }

func TestSuggestFindsTheRightPeople(t *testing.T) {
	cases := map[string]string{
		"Performanceoptimierung relationaler Datenbanken":                  "Brandt, Jonas",
		"Migration einer Anwendung nach Kubernetes mit CI/CD-Pipeline":     "Varga, Ilse",
		"Penetrationstest der Kundenportale und Verschlüsselung":           "Okafor, Mira",
		"Einsatz von Large Language Models zur Klassifikation von Tickets": "Kurz, Lea",
		"KPI-gestütztes Controlling der IT-Kosten":                         "Engel, Paul",
		"Social-Media-Kampagnen im B2B-Vertrieb messen":                    "Weiss, Hanna",
	}
	for topic, want := range cases {
		rs := Suggest(topic, people, 3)
		if len(rs) == 0 || top(rs) != want {
			got := ""
			for _, r := range rs {
				got += r.Reviewer.Name + " " + string(rune('0'+r.Percent/10)) + "0% · "
			}
			t.Errorf("%q: top %q, want %q (%s)", topic, func() string {
				if len(rs) > 0 {
					return top(rs)
				}
				return "-"
			}(), want, got)
		}
	}
}

func TestMatchExplainsItself(t *testing.T) {
	m := MatchOne("Datenbankperformance in der Cloud", people[0])
	if m.Percent < 40 {
		t.Errorf("Brandt for a database topic: %d %%", m.Percent)
	}
	found := false
	for _, x := range m.Matched {
		if x.Area == "Datenbanksysteme" {
			found = true
		}
	}
	if !found {
		t.Errorf("the matching area must be named: %+v", m.Matched)
	}
	// the cloud part is not Brandt's: it shows up as uncovered
	if !strings.Contains(strings.Join(m.Uncovered, " "), "Cloud") {
		t.Errorf("uncovered %v", m.Uncovered)
	}
	// someone from another field scores low
	if o := MatchOne("Datenbankperformance in der Cloud", people[3]); o.Percent >= m.Percent || o.Percent > 25 {
		t.Errorf("marketing for a database topic: %d %%", o.Percent)
	}
}

func TestWorkloadBreaksTies(t *testing.T) {
	two := []Reviewer{
		{Name: "Voll, Vera", Areas: []string{"Datenbanksysteme"}, Load: 2},
		{Name: "Frei, Fritz", Areas: []string{"Datenbanksysteme"}, Load: 0},
	}
	if rs := Suggest("Datenbanksysteme", two, 2); top(rs) != "Frei, Fritz" {
		t.Errorf("same fit: the one with capacity first, got %s", top(rs))
	}
}

func TestEmptyTopic(t *testing.T) {
	if rs := Suggest("  ", people, 3); len(rs) != 0 {
		t.Errorf("no topic, no suggestions: %+v", rs)
	}
}

// a word many reviewers share ("data") must not outweigh the specific one
func TestRareTermsWeighMore(t *testing.T) {
	rs := []Reviewer{
		{Name: "Datenbank, Dora", Areas: []string{"Datenbanksysteme", "Data Warehouse"}, Load: 0},
		{Name: "Marke, Mia", Areas: []string{"Marketing", "Data Analytics"}, Load: 0},
		{Name: "Analyse, Anna", Areas: []string{"Data Science"}, Load: 0},
		{Name: "Daten, Dirk", Areas: []string{"Data Visualisierung"}, Load: 0},
	}
	if got := top(Suggest("Datenqualität im Data Warehouse", rs, 3)); got != "Datenbank, Dora" {
		t.Errorf("top %s", got)
	}
}

func TestBulletsAreCleaned(t *testing.T) {
	if got := cleanArea("• Projektmanagement, Agilität"); got != "Projektmanagement, Agilität" {
		t.Errorf("%q", got)
	}
}

func TestUncoveredShowsTheWordsAsWritten(t *testing.T) {
	m := MatchOne("Migration nach Kubernetes", people[3])
	if got := strings.Join(m.Uncovered, ","); got != "Migration,Kubernetes" {
		t.Errorf("uncovered %q", got)
	}
}

func TestPersonnelTopicsFindHR(t *testing.T) {
	rs := []Reviewer{
		{Name: "Pohl", Areas: []string{"Personalmanagement, Organisation"}},
		{Name: "Scheffer", Areas: []string{"Kundenbindung", "Marketing"}},
	}
	if got := Suggest("Mitarbeiterbindung von Generation Z im Mittelstand", rs, 2); top(got) != "Pohl" {
		t.Fatalf("top %q: %+v", top(got), got)
	}
}
