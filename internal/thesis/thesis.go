// Package thesis is the bachelor thesis at the NORDAKADEMIE: the CIS pages
// (Bachelorthesis, Anmeldung, Übersicht der Gutachtenden), the planning aid of
// the cohort, and the dates that follow from the rules:
//
//   - topic after the lecture period of semester 6, with all module exams up
//     to semester 4 and Transferleistungen 1–5 passed (PO § 7 Abs. 1);
//     Transferleistung 6 uploaded at the latest with the thesis
//   - two months from the approval (PO § 7 Abs. 2, PVO § 22 Abs. 2); at most
//     five weeks more on application, for reasons not one's own
//   - review within four lecture weeks (PVO § 17 Abs. 3; "Dauer Gutachten:
//     4 Vorlesungswochen" on the CIS page)
//   - the grades must be in by the "letzte Noten" date of a graduation
//
// The planning aid of cohort 2023 shows the timing of the examination office:
// registration in a week, start on the Monday two weeks later (KW 50 →
// 21.12.2026), submission two months on, moved off a weekend (22.02.2027).
package thesis

import (
	"html"
	"regexp"
	"strings"
	"time"
)

const (
	Months          = 2
	ExtensionWeeks  = 5
	ReviewWeeks     = 4
	StartAfterWeeks = 2 // from the Monday of the registration week
	TLWeeks         = 9 // a Transferleistung from Auftragsklärung to assessment
)

type Quarter struct {
	Name     string
	From, To time.Time
}

type Deadline struct {
	Name       string    // "März 2027"
	LastGrades time.Time // letzte Noten
	Board      time.Time // Prüfungsausschuss = Entlassung
	Ceremony   string    // "Bachelor-Graduierung: 16.04.2027"
}

func de(t time.Time) string { return t.Format("02.01.2006") }

func monday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// a deadline on a weekend moves to the Monday
func workday(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, 2)
	case time.Sunday:
		return t.AddDate(0, 0, 1)
	}
	return t
}

// StartFor: the processing time of a registration starts on the Monday two
// weeks after the registration week (planning aid: 10.10. → 19.10.).
func StartFor(registration time.Time) time.Time {
	return monday(registration).AddDate(0, 0, 7*StartAfterWeeks)
}

func lectureWeek(w time.Time, quarters []Quarter) bool {
	end := w.AddDate(0, 0, 6)
	for _, q := range quarters {
		if !end.Before(q.From) && !w.After(q.To) {
			return true
		}
	}
	return false
}

// reviewUntil counts four lecture weeks from the submission; weeks without
// lectures (Christmas, Easter) do not count.
func reviewUntil(sub time.Time, quarters []Quarter) time.Time {
	w, n := sub, 0
	for i := 0; n < ReviewWeeks && i < 60; i++ {
		if lectureWeek(w, quarters) {
			n++
		}
		w = w.AddDate(0, 0, 7)
	}
	return w
}

type Graduation struct {
	Name       string `json:"name"`
	LastGrades string `json:"last_grades"`
	Board      string `json:"board"`
	Ceremony   string `json:"ceremony,omitempty"`
}

type Result struct {
	Start         string      `json:"start"`
	Submission    string      `json:"submission"`     // last day, 23:59; the postmark counts
	WithExtension string      `json:"with_extension"` // if the full five weeks are granted
	ReviewUntil   string      `json:"review_until"`   // four lecture weeks after the submission
	Graduation    *Graduation `json:"graduation,omitempty"`
	BufferDays    int         `json:"buffer_days"`
	ExtGraduation *Graduation `json:"graduation_with_extension,omitempty"`
}

func graduationFor(t time.Time, deadlines []Deadline) (*Graduation, int) {
	for _, d := range deadlines {
		if !d.LastGrades.Before(t) {
			return &Graduation{d.Name, de(d.LastGrades), de(d.Board), d.Ceremony}, int(d.LastGrades.Sub(t).Hours() / 24)
		}
	}
	return nil, 0
}

// Calc: what follows from a start date.
func Calc(start time.Time, quarters []Quarter, deadlines []Deadline) Result {
	sub := workday(start.AddDate(0, Months, 0))
	ext := workday(sub.AddDate(0, 0, 7*ExtensionWeeks))
	rev := reviewUntil(sub, quarters)
	r := Result{Start: de(start), Submission: de(sub), WithExtension: de(ext), ReviewUntil: de(rev)}
	r.Graduation, r.BufferDays = graduationFor(rev, deadlines)
	r.ExtGraduation, _ = graduationFor(reviewUntil(ext, quarters), deadlines)
	return r
}

type LatestStart struct {
	Graduation       Graduation `json:"graduation"`
	Start            string     `json:"start"`
	Submission       string     `json:"submission"`
	ReviewUntil      string     `json:"review_until"`
	RegisterWeekFrom string     `json:"register_week_from"`
	RegisterWeekTo   string     `json:"register_week_to"`
	KW               int        `json:"kw"`
	TLStartBy        string     `json:"tl_start_by"` // the last of T1–T5 must be passed by the registration
	Found            bool       `json:"found"`
}

// Latest: the last start (a Monday) whose review still ends by the deadline's
// "letzte Noten", and the registration week that leads to it.
func Latest(d Deadline, quarters []Quarter, deadlines []Deadline) LatestStart {
	out := LatestStart{Graduation: Graduation{d.Name, de(d.LastGrades), de(d.Board), d.Ceremony}}
	for s := monday(d.LastGrades); s.After(d.LastGrades.AddDate(-1, 0, 0)); s = s.AddDate(0, 0, -7) {
		sub := workday(s.AddDate(0, Months, 0))
		rev := reviewUntil(sub, quarters)
		if rev.After(d.LastGrades) {
			continue
		}
		reg := s.AddDate(0, 0, -7*StartAfterWeeks)
		_, kw := reg.ISOWeek()
		out.Start, out.Submission, out.ReviewUntil = de(s), de(sub), de(rev)
		out.RegisterWeekFrom, out.RegisterWeekTo, out.KW = de(reg), de(reg.AddDate(0, 0, 6)), kw
		out.TLStartBy = de(reg.AddDate(0, 0, 6-7*TLWeeks))
		out.Found = true
		return out
	}
	return out
}

// ── the planning aid of a cohort (PDF linked on the Bachelorthesis page) ───

type PlanningAid struct {
	Found                bool   `json:"found"`
	Cohort               string `json:"cohort,omitempty"`
	EarliestRegistration string `json:"earliest_registration,omitempty"`
	EarliestStart        string `json:"earliest_start,omitempty"`
	Semester7From        string `json:"semester7_from,omitempty"`
	Semester7To          string `json:"semester7_to,omitempty"`
	LatestRegisterKW     string `json:"latest_register_kw,omitempty"`
	LatestRegisterFrom   string `json:"latest_register_from,omitempty"`
	LatestRegisterTo     string `json:"latest_register_to,omitempty"`
	LatestStart          string `json:"latest_start,omitempty"`
	LatestSubmission     string `json:"latest_submission,omitempty"`
	RepeatFrom           string `json:"repeat_exams_from,omitempty"`
	RepeatTo             string `json:"repeat_exams_to,omitempty"`
	LastGrades           string `json:"last_grades,omitempty"`
	Board                string `json:"board,omitempty"`
	Ceremony             string `json:"ceremony,omitempty"`
	URL                  string `json:"url,omitempty"`
}

const dateRe = `(\d{2}\.\d{2}\.\d{4})`
const dayRe = `(\d{2}\.\d{2}\.)`

var (
	spaces   = regexp.MustCompile(`\s+`)
	aidRules = []struct {
		re  *regexp.Regexp
		set func(*PlanningAid, []string)
	}{
		{regexp.MustCompile(`Jahrgang (\d{4})`), func(p *PlanningAid, m []string) { p.Cohort = m[1] }},
		{regexp.MustCompile(`Früheste Anmeldemöglichkeit Bachelorthesis: ` + dateRe), func(p *PlanningAid, m []string) { p.EarliestRegistration = m[1] }},
		{regexp.MustCompile(`frühester Start: ` + dateRe), func(p *PlanningAid, m []string) { p.EarliestStart = m[1] }},
		{regexp.MustCompile(`7\. Theoriesemester[^:]*: ` + dayRe + ` ?[–-] ?` + dateRe), func(p *PlanningAid, m []string) {
			p.Semester7From, p.Semester7To = withYear(m[1], m[2]), m[2]
		}},
		{regexp.MustCompile(`KW (\d+): ` + dayRe + ` ?[–-] ?` + dateRe), func(p *PlanningAid, m []string) {
			p.LatestRegisterKW, p.LatestRegisterFrom, p.LatestRegisterTo = m[1], withYear(m[2], m[3]), m[3]
		}},
		{regexp.MustCompile(`spätester Start: ` + dateRe), func(p *PlanningAid, m []string) { p.LatestStart = m[1] }},
		{regexp.MustCompile(`späteste Abgabe: ` + dateRe), func(p *PlanningAid, m []string) { p.LatestSubmission = m[1] }},
		{regexp.MustCompile(`Wiederholungsklausuren[^:]*: ` + dayRe + ` ?[–-] ?` + dateRe), func(p *PlanningAid, m []string) {
			p.RepeatFrom, p.RepeatTo = withYear(m[1], m[2]), m[2]
		}},
		{regexp.MustCompile(`zu liefern am: ` + dateRe), func(p *PlanningAid, m []string) { p.LastGrades = m[1] }},
		{regexp.MustCompile(`Termin Prüfungsausschuss: ` + dateRe), func(p *PlanningAid, m []string) { p.Board = m[1] }},
		{regexp.MustCompile(`Verabschiedung: ` + dateRe), func(p *PlanningAid, m []string) { p.Ceremony = m[1] }},
	}
)

// "26.10." with "13.11.2026": the year of the end, one less if the month is later
func withYear(dm, full string) string {
	if len(dm) < 6 || len(full) < 10 {
		return dm
	}
	y := full[6:]
	if dm[3:5] > full[3:5] {
		if n, err := time.Parse("2006", y); err == nil {
			y = n.AddDate(-1, 0, 0).Format("2006")
		}
	}
	return dm + y
}

func ParsePlanningAid(text string) PlanningAid {
	t := spaces.ReplaceAllString(text, " ")
	var p PlanningAid
	for _, r := range aidRules {
		if m := r.re.FindStringSubmatch(t); m != nil {
			r.set(&p, m)
		}
	}
	p.Found = p.EarliestRegistration != "" || p.LatestStart != ""
	return p
}

// ── Anmeldung Bachelorthesis: may the owner register now? ─────────────────

type Eligibility struct {
	Open    bool     `json:"open"`
	Reasons []string `json:"reasons"` // the CIS's own words, e.g. "Es wurden nicht alle benötigten Transferleistungen bestanden."
	Known   bool     `json:"known"`
}

var (
	applyBlock = regexp.MustCompile(`(?s)tx-na-applyforthesis(.*)`)
	dangerRe   = regexp.MustCompile(`(?s)<li class="[^"]*alert-danger[^"]*">(.*?)</li>`)
	tagRe      = regexp.MustCompile(`<[^>]+>`)
)

func text(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(s, " ")), " "))
}

func ParseEligibility(body string) Eligibility {
	e := Eligibility{Reasons: []string{}}
	m := applyBlock.FindStringSubmatch(body)
	if m == nil {
		return e
	}
	e.Known = true
	for _, r := range dangerRe.FindAllStringSubmatch(m[1], -1) {
		if t := text(r[1]); t != "" {
			e.Reasons = append(e.Reasons, t)
		}
	}
	e.Open = len(e.Reasons) == 0 && strings.Contains(m[1], "<form")
	return e
}

// ── Übersicht der Gutachtenden ─────────────────────────────────────────────

type Reviewer struct {
	Name       string   `json:"name"`
	Email      string   `json:"email,omitempty"`
	Department string   `json:"department,omitempty"`
	Areas      []string `json:"areas"`
	Load       int      `json:"load"` // 0 frei, 1 mittel, 2 voll, -1 unknown
	LoadLabel  string   `json:"load_label,omitempty"`
}

var (
	rowRe     = regexp.MustCompile(`(?s)<tr.*?</tr>`)
	cellRe    = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	mailRe    = regexp.MustCompile(`mailto:([^"]+)"[^>]*>(.*?)</a>`)
	checkedRe = regexp.MustCompile(`value="(\d)"[^>]*checked`)
	brRe      = regexp.MustCompile(`<br\s*/?>`)
	// a line ending like this continues on the next one
	// "Masterstudiengänge:" and the like structure the list, they are no areas
	sectionHead = regexp.MustCompile(`:\s*$`)
	// a line that is only an adjective ("Allgemeine") belongs to the next one
	danglingAdj = regexp.MustCompile(`^\p{Lu}[\p{Ll}-]+(ische|liche|ale|eine|ige|ive|elle|ene|ere)$`)
	joinEnd     = regexp.MustCompile(`(?i)(\bund|\bsowie|\boder|\bder|\bdie|\bdes|\bvon|\bzu|\bfür|\bim|\bin|\bmit|&|-)$`)
)

var loadLabels = map[int]string{0: "frei", 1: "mittel", 2: "voll"}

func ParseReviewers(body string) []Reviewer {
	var out []Reviewer
	for _, row := range rowRe.FindAllString(body, -1) {
		cells := cellRe.FindAllStringSubmatch(row, -1)
		if len(cells) < 3 {
			continue
		}
		r := Reviewer{Load: -1, Areas: []string{}}
		if m := mailRe.FindStringSubmatch(cells[0][1]); m != nil {
			r.Email, r.Name = html.UnescapeString(m[1]), text(m[2])
		} else {
			r.Name = text(cells[0][1])
		}
		if r.Name == "" {
			continue
		}
		r.Department = text(cells[1][1])
		var cur string
		for _, line := range brRe.Split(cells[2][1], -1) {
			l := text(line)
			if l == "" || sectionHead.MatchString(l) {
				continue
			}
			if cur != "" {
				cur += " " + l
			} else {
				cur = l
			}
			if !joinEnd.MatchString(cur) && !danglingAdj.MatchString(cur) {
				r.Areas = addArea(r.Areas, cleanArea(cur))
				cur = ""
			}
		}
		if cur != "" {
			r.Areas = addArea(r.Areas, cleanArea(cur))
		}
		if len(cells) > 3 {
			if m := checkedRe.FindStringSubmatch(cells[3][1]); m != nil {
				r.Load = int(m[1][0] - '0')
				r.LoadLabel = loadLabels[r.Load]
			}
		}
		out = append(out, r)
	}
	return out
}

func addArea(as []string, a string) []string {
	for _, x := range as {
		if strings.EqualFold(x, a) {
			return as
		}
	}
	return append(as, a)
}

// cleanArea drops list bullets and a trailing comma
func cleanArea(a string) string {
	a = strings.TrimSpace(strings.TrimLeft(a, "•·-–* "))
	return strings.TrimSpace(strings.TrimSuffix(a, ","))
}

// FilterReviewers: every word must appear in name, department or an area.
func FilterReviewers(rs []Reviewer, query string) []Reviewer {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return rs
	}
	var out []Reviewer
	for _, r := range rs {
		hay := strings.ToLower(r.Name + " " + r.Department + " " + strings.Join(r.Areas, " "))
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}
