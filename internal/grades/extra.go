package grades

import (
	"strings"
	"time"
)

const SourceExtra = "weitere_leistungen"

// applyExtra lays results from "weitere Leistungen" over the module they
// belong to. An exam can run under another number than the module (new exam
// regulations: I222 for I168); the CIS then books the result as an extra and
// leaves the module's own row at the earlier attempt, so the title links them.
// A passed module is never touched, a failed extra only if it is the newer result.
func (o *Overview) applyExtra() {
	for i := range o.Extra {
		e := &o.Extra[i]
		if e.Status == StatusOpen {
			continue
		}
		m := o.moduleFor(e)
		if m == nil || m.Status == StatusPassed {
			continue
		}
		if e.Status == StatusFailed && m.Grade != "" && !entered(e).After(entered(m)) {
			continue
		}
		attempt := e.Attempt
		if m.Status == StatusFailed && attempt <= m.Attempt {
			attempt = m.Attempt + 1
		}
		m.Superseded, m.RecordedAs, m.Source = m.Grade, e.ModuleNr, SourceExtra
		m.Grade, m.GradeValue, m.Status, m.Attempt = e.Grade, e.GradeValue, e.Status, attempt
		m.Recognized, m.OralAllowed = e.Recognized, e.OralAllowed
		// dates and distribution belong to the replaced attempt
		m.ExamDate, m.EntryDate = e.ExamDate, e.EntryDate
		m.PerformanceID, m.StatisticURL = e.PerformanceID, e.StatisticURL
		if e.Credits != "" {
			m.Credits = e.Credits
		}
		e.CountsFor = m.ModuleNr
	}
}

func (o *Overview) moduleFor(e *ModuleGrade) *ModuleGrade {
	for i := range o.Modules {
		if SameModule(o.Modules[i].ModuleNr, e.ModuleNr) {
			return &o.Modules[i]
		}
	}
	key := TitleKey(e.Title)
	for i := range o.Modules {
		if key != "" && TitleKey(o.Modules[i].Title) == key {
			return &o.Modules[i]
		}
	}
	return nil
}

func entered(m *ModuleGrade) time.Time {
	t, _ := time.Parse("02.01.2006", strings.TrimSpace(m.EntryDate))
	return t
}

var roman = map[string]string{"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6"}

// TitleKey makes module titles comparable: case, spacing, and "II" for "2"
// (the CIS uses both for the same module in different lists).
func TitleKey(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if n, ok := roman[w]; ok && i > 0 {
			words[i] = n
		}
	}
	return strings.Join(words, " ")
}
