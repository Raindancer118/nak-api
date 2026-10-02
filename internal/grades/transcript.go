package grades

import (
	"regexp"
	"strings"

	"github.com/Raindancer118/nak-api/internal/drift"
)

// The Notenübersicht PDF is generated on request and knows a new grade before
// the Leistungsübersicht page shows it, sometimes by twenty minutes.

type TranscriptGrade struct {
	ModuleNr string `json:"module_nr"`
	Title    string `json:"title"`
	Grade    string `json:"grade"`
	Credits  string `json:"credits,omitempty"`
}

type TranscriptGrades struct {
	Modules []TranscriptGrade `json:"modules"`
	Average string            `json:"average,omitempty"`
	Date    string            `json:"date,omitempty"` // as printed: Datum der Ausstellung
}

var (
	rowRe     = regexp.MustCompile(`^([A-Z]{1,3}\d{3})\s+(.*)$`)
	colsRe    = regexp.MustCompile(`\s{2,}`)
	creditsRe = regexp.MustCompile(`^(\d+(?:,\d+)?) CP$`)
	averageRe = regexp.MustCompile(`Notendurchschnitt ist\s+(\d,\d+)`)
	issuedRe  = regexp.MustCompile(`Ausstellung der Notenübersicht:\s*(\d{2}\.\d{2}\.\d{4})`)
)

// ParseTranscriptText reads the text of the Notenübersicht (pdftotext -layout):
// one row per module with title, grade and credits; "#" marks modules
// without an exam yet, long titles wrap with the grade on the next line.
func ParseTranscriptText(text string) (*TranscriptGrades, error) {
	if !strings.Contains(text, "ModulNr") {
		return nil, drift.New("Notenübersicht (PDF)", "table header ModulNr / Name / Note / Credits", text)
	}
	out := &TranscriptGrades{Modules: []TranscriptGrade{}}
	var open *TranscriptGrade // row whose grade is still to come
	for _, line := range strings.Split(text, "\n") {
		if m := rowRe.FindStringSubmatch(strings.TrimRight(line, " ")); m != nil {
			g := TranscriptGrade{ModuleNr: m[1]}
			done := fill(&g, colsRe.Split(strings.TrimSpace(m[2]), -1))
			if done {
				out.add(g)
				open = nil
			} else {
				open = &g
			}
			continue
		}
		if open != nil && strings.HasPrefix(line, " ") && strings.TrimSpace(line) != "" {
			cols := colsRe.Split(strings.TrimSpace(line), -1)
			cols[0] = open.Title + " " + cols[0]
			if fill(open, cols) {
				out.add(*open)
			}
			open = nil
			continue
		}
		open = nil
	}
	if m := averageRe.FindStringSubmatch(text); m != nil {
		out.Average = m[1]
	}
	if m := issuedRe.FindStringSubmatch(text); m != nil {
		out.Date = m[1]
	}
	return out, nil
}

// fill takes [title, grade?, credits?]; false means the grade column is
// missing (the title wraps onto the next line).
func fill(g *TranscriptGrade, cols []string) bool {
	g.Title = strings.TrimSpace(cols[0])
	if len(cols) < 2 {
		return false
	}
	g.Grade = strings.TrimSpace(cols[1])
	if len(cols) > 2 {
		if m := creditsRe.FindStringSubmatch(strings.TrimSpace(cols[2])); m != nil {
			g.Credits = m[1]
		}
	}
	return true
}

func (t *TranscriptGrades) add(g TranscriptGrade) {
	if g.Grade == "" || g.Grade == "#" {
		return
	}
	t.Modules = append(t.Modules, g)
}

var spaceRe = regexp.MustCompile(`\s+`)

// NormalizeGrade makes the PDF's "4,0 (2. Versuch)" and the page's
// "4,0 (2.Versuch)" the same value; markers like "*" (recognised) go.
func NormalizeGrade(g string) string {
	g = strings.ToLower(spaceRe.ReplaceAllString(g, ""))
	return strings.TrimRight(g, "*⁂")
}
