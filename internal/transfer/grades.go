package transfer

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Transferleistungen are Studienleistungen of the Transfermodule Theorie/Praxis
// 1–6 (PVO § 18): not graded, only "bestanden"/"nicht bestanden", never part
// of the overall grade. Each is written on a topic of a module of the
// required curriculum. The assessment still grades its criteria; their
// weighted value is shown for orientation, labelled as no official grade.

type TransferGrade struct {
	ID          string  `json:"id"`
	No          string  `json:"no"`
	Topic       string  `json:"topic"`
	Module      string  `json:"module"`    // the module the topic belongs to, "Name (I162)"
	ModuleNr    string  `json:"module_nr"` // I162
	Abgabedatum string  `json:"abgabedatum"`
	Wertung     string  `json:"wertung"`
	Versuch     string  `json:"versuch"`
	Exact       float64 `json:"exact"`        // weighted mean of the criteria
	Grade       string  `json:"grade"`        // the same, two decimals ("2,55"); no official grade
	Final       string  `json:"final"`        // as a composite grade would be formed: one decimal, rounded down (PVO § 17 Abs. 4)
	Criteria    int     `json:"criteria"`     // criteria with a grade and a weight
	MustPassOK  bool    `json:"must_pass_ok"` // every "muss bestanden werden" criterion at 4,0 or better
}

type TransferGrades struct {
	Items   []TransferGrade `json:"items"`
	Average string          `json:"average,omitempty"` // mean of the criteria values
	Final   string          `json:"final,omitempty"`   // the mean as a grade, like FinalGrade
	Count   int             `json:"count"`
	Note    string          `json:"note"`
}

const GradesNote = "Rechnerisch, nicht vom CIS: Transferleistungen sind Studienleistungen und werden offiziell nur mit „bestanden“ oder „nicht bestanden“ bewertet (PVO § 18); sie gehen nicht in die Gesamtnote ein. Kriterienwert = gewichteter Mittelwert der Bewertungskriterien; die Note daraus ist wie bei zusammengesetzten Noten auf eine Nachkommastelle abgerundet (PVO § 17 Abs. 4)."

// CriteriaValue shows the weighted value with two decimals, rounded half up
// (2.225 is 2,23 even though the float is 2.22499…).
func CriteriaValue(x float64) string {
	return strings.Replace(fmt.Sprintf("%.2f", math.Floor(x*100+0.5+1e-6)/100), ".", ",", 1)
}

// FinalGrade is the Transferleistung's grade as if it were graded: like a
// composite grade, one decimal, rounded down (PVO § 17 Abs. 4); 2.55 is 2,5.
// Tolerant of float noise (2.6999999 is 2,7). The CIS shows no such grade.
func FinalGrade(x float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", math.Floor(x*10+1e-6)/10), ".", ",", 1)
}

var moduleNr = regexp.MustCompile(`\(([A-Z]{1,3}\d{3})\)\s*$`)

// ComputeGrades turns the assessed reports into grades; reports without an
// assessment (still open) are left out.
func ComputeGrades(reports []Report, bew map[string]*Bewertung) TransferGrades {
	out := TransferGrades{Items: []TransferGrade{}, Note: GradesNote}
	var sum float64
	for _, r := range reports {
		b := bew[r.ID]
		if b == nil {
			continue
		}
		exact, ok := WeightedAverage(b.Kriterien)
		if !ok {
			continue
		}
		g := TransferGrade{ID: r.ID, No: r.No, Topic: r.Topic, Module: r.Module, Abgabedatum: r.Abgabedatum, Wertung: r.Wertung,
			Versuch: r.Versuch, Exact: exact, Grade: CriteriaValue(exact), Final: FinalGrade(exact), MustPassOK: true}
		if m := moduleNr.FindStringSubmatch(r.Module); m != nil {
			g.ModuleNr = m[1]
		}
		for _, k := range b.Kriterien {
			note, ok1 := parseGermanFloat(k.Note)
			if _, ok2 := parsePercent(k.Gewichtung); ok1 && ok2 {
				g.Criteria++
			}
			if ok1 && strings.Contains(strings.ToLower(k.Kriterium), "muss bestanden werden") && note > 4.0 {
				g.MustPassOK = false
			}
		}
		out.Items = append(out.Items, g)
		sum += exact
	}
	out.Count = len(out.Items)
	if out.Count > 0 {
		out.Average = CriteriaValue(sum / float64(out.Count))
		out.Final = FinalGrade(sum / float64(out.Count))
	}
	return out
}
