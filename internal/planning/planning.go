// Package planning reads study-planning pages: the personal Studienplan
// (modules × semesters with exam forms), lecture periods (Vorlesungszeiten)
// and graduation deadlines (Abschlussfristen on the start page).
package planning

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

const (
	StudienplanPath      = "/mein-profil/meine-planung/mein-studienplan"
	VorlesungszeitenPath = "/studium/bachelor/vorlesungszeiten"
	StartPath            = "/"
)

type SemesterLoad struct {
	Semester int `json:"semester"`
	Hours    int `json:"hours"` // Semesterwochenstunden-artige Angabe aus dem Plan
}

type Module struct {
	ModuleNr     string         `json:"module_nr"`
	Title        string         `json:"title"`
	Group        string         `json:"group"`
	Semesters    []SemesterLoad `json:"semesters"`
	ExamSemester int            `json:"exam_semester,omitempty"`
	ExamForm     string         `json:"exam_form,omitempty"`
	ExamFormName string         `json:"exam_form_name,omitempty"`
	Credits      int            `json:"credits"`
}

func (m Module) TotalHours() int {
	n := 0
	for _, s := range m.Semesters {
		n += s.Hours
	}
	return n
}

type Studienplan struct {
	Modules []Module          `json:"modules"`
	Legend  map[string]string `json:"legend"`
}

func (s *Studienplan) TotalCredits() int {
	n := 0
	for _, m := range s.Modules {
		n += m.Credits
	}
	return n
}

// BySemester returns the modules with load or exam in semester n.
func (s *Studienplan) BySemester(n int) []Module {
	var out []Module
	for _, m := range s.Modules {
		hit := m.ExamSemester == n
		for _, l := range m.Semesters {
			hit = hit || l.Semester == n
		}
		if hit {
			out = append(out, m)
		}
	}
	return out
}

func FetchStudienplan(c *client.Client) (*Studienplan, error) {
	p, err := c.Page(StudienplanPath)
	if err != nil {
		return nil, err
	}
	sp := ParseStudienplan(p.Body)
	if len(sp.Modules) == 0 {
		return nil, fmt.Errorf("studienplan: no modules found")
	}
	return sp, nil
}

var (
	cellRe   = regexp.MustCompile(`^(\d+)?\s*([A-Za-z]+)?$`)
	legendRe = regexp.MustCompile(`([A-Z][A-Za-z]?)\s*=\s*([^,]+)`)
)

func ParseStudienplan(body string) *Studienplan {
	doc := htmlx.MustParse(body)
	sp := &Studienplan{Legend: map[string]string{}}
	tbl := htmlx.First(doc, htmlx.Class("studienplan"))
	if tbl == nil {
		return sp
	}
	group := ""
	for _, tr := range htmlx.All(tbl, htmlx.Tag("tr")) {
		if g := htmlx.First(tr, htmlx.Class("modulgruppe")); g != nil {
			group = htmlx.Text(g)
			continue
		}
		nr := htmlx.First(tr, htmlx.Class("modulnr"))
		if nr == nil {
			continue
		}
		m := Module{ModuleNr: htmlx.Text(nr), Title: htmlx.Text(htmlx.First(tr, htmlx.Class("modulbezeichnung"))), Group: group}
		cells := htmlx.Children(tr, htmlx.Tag("td"))
		// cells: Modulnr | Bezeichnung | one cell per semester (class stunden/klausur/…) | CP
		if len(cells) < 4 {
			continue
		}
		for i, td := range cells[2 : len(cells)-1] {
			sem := i + 1
			mm := cellRe.FindStringSubmatch(htmlx.Text(td))
			if mm == nil {
				continue
			}
			if mm[1] != "" {
				h, _ := strconv.Atoi(mm[1])
				m.Semesters = append(m.Semesters, SemesterLoad{Semester: sem, Hours: h})
			}
			if mm[2] != "" {
				m.ExamSemester, m.ExamForm = sem, mm[2]
			}
		}
		if len(cells) > 0 {
			m.Credits, _ = strconv.Atoi(htmlx.Text(cells[len(cells)-1]))
		}
		sp.Modules = append(sp.Modules, m)
	}
	for _, m := range legendRe.FindAllStringSubmatch(htmlx.Text(htmlx.Main(doc)), -1) {
		sp.Legend[m[1]] = strings.TrimSpace(m[2])
	}
	for i := range sp.Modules {
		sp.Modules[i].ExamFormName = examFormName(sp.Modules[i].ExamForm, sp.Legend)
	}
	return sp
}

func examFormName(code string, legend map[string]string) string {
	if v, ok := legend[code]; ok {
		return v
	}
	switch code {
	case "Pf":
		return "Portfolio/Prüfung über mehrere Semester"
	case "B":
		return "Bachelorarbeit"
	}
	return ""
}

// ── Vorlesungszeiten ────────────────────────────────────────────────────────

type Quarter struct {
	Name string `json:"name"` // e.g. "III/26"
	From string `json:"from"`
	To   string `json:"to"`
}

var rangeRe = regexp.MustCompile(`(\d{2}\.\d{2}\.\d{4})\s*-\s*(\d{2}\.\d{2}\.\d{4})`)

func FetchVorlesungszeiten(c *client.Client) ([]Quarter, error) {
	p, err := c.Page(VorlesungszeitenPath)
	if err != nil {
		return nil, err
	}
	return ParseVorlesungszeiten(p.Body), nil
}

func ParseVorlesungszeiten(body string) []Quarter {
	doc := htmlx.Main(htmlx.MustParse(body))
	var out []Quarter
	for _, tbl := range htmlx.All(doc, htmlx.Tag("table")) {
		var names []string
		for _, tr := range htmlx.All(tbl, htmlx.Tag("tr")) {
			cells := htmlx.Cells(tr)
			if len(cells) == 0 {
				continue
			}
			if htmlx.First(tr, htmlx.Tag("strong", "h2", "h3", "h4", "b")) != nil && !rangeRe.MatchString(htmlx.Text(tr)) {
				names = names[:0]
				for _, c := range cells {
					names = append(names, htmlx.Text(c))
				}
				continue
			}
			for i, c := range cells {
				m := rangeRe.FindStringSubmatch(strings.ReplaceAll(htmlx.Text(c), " ", " "))
				if m == nil || i >= len(names) {
					continue
				}
				out = append(out, Quarter{Name: names[i], From: m[1], To: m[2]})
			}
		}
	}
	return out
}

// Current returns the quarter containing now (nil between quarters) and the next one.
func Current(qs []Quarter, now time.Time) (cur, next *Quarter) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for i := range qs {
		from, e1 := time.Parse("02.01.2006", qs[i].From)
		to, e2 := time.Parse("02.01.2006", qs[i].To)
		if e1 != nil || e2 != nil {
			continue
		}
		if !day.Before(from) && !day.After(to) {
			cur = &qs[i]
		} else if day.Before(from) && next == nil {
			next = &qs[i]
		}
	}
	return cur, next
}

// ── Abschlussfristen (start page) ───────────────────────────────────────────

type Frist struct {
	Abschluss          string `json:"abschluss"`
	LetzteNoten        string `json:"letzte_noten"`
	Pruefungsausschuss string `json:"pruefungsausschuss_entlassung"`
	Graduierung        string `json:"graduierung,omitempty"`
	Text               string `json:"text"`
}

var (
	fristAbschluss = regexp.MustCompile(`Abschluss im\s+(.+?)\s*:`)
	fristNoten     = regexp.MustCompile(`Noten müssen am\s+(\d{2}\.\d{2}\.\d{4})`)
	fristPA        = regexp.MustCompile(`Prüfungsausschusssitzung ist am\s+(\d{2}\.\d{2}\.\d{4})`)
	fristGrad      = regexp.MustCompile(`((?:Bachelor|Master)-Graduierung:\s*\d{2}\.\d{2}\.\d{4})`)
)

func FetchAbschlussfristen(c *client.Client) ([]Frist, error) {
	p, err := c.Page(StartPath)
	if err != nil {
		return nil, err
	}
	return ParseAbschlussfristen(p.Body), nil
}

func ParseAbschlussfristen(body string) []Frist {
	doc := htmlx.MustParse(body)
	h := htmlx.First(doc, func(n *html.Node) bool {
		return htmlx.Tag("h2", "h3")(n) && strings.HasPrefix(htmlx.Text(n), "Abschlussfristen")
	})
	if h == nil {
		return nil
	}
	var out []Frist
	// The paragraphs follow the heading inside the same content element.
	for box := h.Parent; box != nil; box = box.Parent {
		ps := htmlx.All(box, htmlx.Tag("p"))
		for _, p := range ps {
			t := strings.ReplaceAll(htmlx.Text(p), " ", " ")
			m := fristAbschluss.FindStringSubmatch(t)
			if m == nil {
				continue
			}
			f := Frist{Abschluss: m[1], Text: t}
			if x := fristNoten.FindStringSubmatch(t); x != nil {
				f.LetzteNoten = x[1]
			}
			if x := fristPA.FindStringSubmatch(t); x != nil {
				f.Pruefungsausschuss = x[1]
			}
			if x := fristGrad.FindStringSubmatch(t); x != nil {
				f.Graduierung = x[1]
			}
			out = append(out, f)
		}
		if len(out) > 0 {
			return out
		}
	}
	return out
}
