// Package exams reads the Prüfungsübersicht (tx_naexams, controller
// Pruefungsverwaltung) and performs register/deregister actions.
//
// Each row offers at most one action link ("register" or "deregister"). The
// link is a cHash-signed GET — following it IS the binding action, so it is
// only ever followed through client.WriteGet after explicit confirmation.
package exams

import (
	"fmt"
	"strings"
	"time"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

// PagePath is the personal exam overview; LegacyPath shows the same table
// below the general exam information page.
const (
	PagePath   = "/mein-profil/meine-planung/meine-pruefungen"
	LegacyPath = "/studium/pruefungen/an-und-abmeldung-von-pruefungen"
)

var berlin = mustLoc("Europe/Berlin")

func mustLoc(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return l
}

type Exam struct {
	ExamID      string     `json:"exam_id"`
	Section     string     `json:"section,omitempty"`
	ModuleNr    string     `json:"module_nr"`
	Title       string     `json:"title"`
	Zenturien   []string   `json:"zenturien"`
	Dozenten    []string   `json:"dozenten"`
	Start       string     `json:"start"`
	Ende        string     `json:"ende"`
	Status      string     `json:"status"`
	Registered  bool       `json:"registered"`
	Action      string     `json:"action"`       // "register", "deregister" or ""
	ActionLabel string     `json:"action_label"` // button text
	ActionURL   string     `json:"action_url"`   // cHash-signed; following it is the binding action
	Deadlines   *Deadlines `json:"deadlines,omitempty"`
}

// Deadlines per Prüfungsverfahrensordnung §10/§11 for written exams: sign-up
// opens 25 and closes 10 calendar days before (24:00), deregistration closes 2
// days before (24:00). Computed client-side; the CIS link availability wins.
type Deadlines struct {
	RegisterOpens   string `json:"register_opens"`
	RegisterCloses  string `json:"register_closes"`
	DeregisterUntil string `json:"deregister_until"`
	DaysUntilExam   int    `json:"days_until_exam"`
	Note            string `json:"note"`
}

// FetchList loads the personal overview, falling back to the legacy page.
func FetchList(c *client.Client) ([]Exam, error) {
	return fetch(c, time.Now())
}

func fetch(c *client.Client, now time.Time) ([]Exam, error) {
	var lastErr error
	for _, path := range []string{PagePath, LegacyPath} {
		p, err := c.Page(path)
		if err != nil {
			lastErr = err
			continue
		}
		if list, found := parseExams(p.Body, c.Base, now); found {
			return list, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("fetch exams: %w", lastErr)
	}
	return nil, nil
}

// Resolve fetches the live overview and returns the exam with that ID.
func Resolve(c *client.Client, examID string) (*Exam, error) {
	list, err := FetchList(c)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ExamID == examID {
			e := list[i]
			if e.ActionURL == "" {
				return &e, fmt.Errorf("exam %s (%s) currently offers no action", examID, e.Title)
			}
			return &e, nil
		}
	}
	return nil, fmt.Errorf("exam %s not found in the current overview", examID)
}

// Submit follows the page-provided action link. BINDING — confirm first.
func Submit(c *client.Client, actionURL string) (string, error) {
	p, err := c.WriteGet(actionURL)
	if err != nil {
		return "", fmt.Errorf("submit exam action: %w", err)
	}
	return FlashMessage(p.Body), nil
}

// parseExams returns all rows of every exam table; found=false when the page
// has no Prüfungsübersicht at all.
func parseExams(body, base string, now time.Time) ([]Exam, bool) {
	doc := htmlx.MustParse(body)
	var out []Exam
	found := false
	for _, tbl := range htmlx.All(doc, htmlx.Tag("table")) {
		head := strings.ToLower(htmlx.Text(htmlx.First(tbl, htmlx.Tag("thead"))))
		if head == "" {
			head = strings.ToLower(htmlx.Text(tbl))
		}
		if !strings.Contains(head, "modulnummer") || !strings.Contains(head, "abmelden") {
			continue
		}
		found = true
		section := precedingHeading(tbl)
		for _, tr := range htmlx.Rows(tbl) {
			if e, ok := parseRow(tr, base); ok {
				e.Section = section
				e.Deadlines = computeDeadlines(e.Start, now)
				out = append(out, e)
			}
		}
	}
	return out, found
}

func parseRow(tr *html.Node, base string) (Exam, bool) {
	cells := htmlx.Children(tr, htmlx.Tag("td"))
	if len(cells) < 8 {
		return Exam{}, false
	}
	e := Exam{
		ModuleNr:  htmlx.Text(cells[0]),
		Title:     htmlx.Text(cells[1]),
		Zenturien: spanTexts(cells[2]),
		Dozenten:  spanTexts(cells[3]),
		Start:     htmlx.Text(cells[4]),
		Ende:      htmlx.Text(cells[5]),
		Status:    htmlx.Text(cells[6]),
	}
	e.Registered = strings.Contains(strings.ToLower(e.Status), "angemeldet") && !strings.Contains(strings.ToLower(e.Status), "abgemeldet")
	if a := htmlx.First(cells[7], func(n *html.Node) bool { return htmlx.Tag("a")(n) && htmlx.Attr(n, "href") != "" }); a != nil {
		href := htmlx.AbsURL(base, htmlx.Attr(a, "href"))
		e.ActionURL = href
		e.ActionLabel = htmlx.Text(a)
		e.Action = htmlx.Action(href)
		e.ExamID = htmlx.QueryParam(href, "examId")
	}
	if e.ModuleNr == "" && e.Title == "" {
		return Exam{}, false
	}
	return e, true
}

func precedingHeading(n *html.Node) string {
	for p := n; p != nil; p = p.Parent {
		for s := p.PrevSibling; s != nil; s = s.PrevSibling {
			if h := htmlx.First(s, htmlx.Tag("h2", "h3", "h4")); h != nil {
				return htmlx.Text(h)
			}
		}
	}
	return ""
}

func computeDeadlines(start string, now time.Time) *Deadlines {
	t, err := time.ParseInLocation("02.01.2006 15:04", start, berlin)
	if err != nil {
		return nil // no time → not a written exam with a fixed slot (e.g. Hausarbeit window)
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, berlin)
	endOf := func(d time.Time) string { return d.Add(-time.Minute).Format("02.01.2006 15:04") }
	today := time.Date(now.In(berlin).Year(), now.In(berlin).Month(), now.In(berlin).Day(), 0, 0, 0, 0, berlin)
	return &Deadlines{
		RegisterOpens:   day.AddDate(0, 0, -25).Format("02.01.2006"),
		RegisterCloses:  endOf(day.AddDate(0, 0, -9)),
		DeregisterUntil: endOf(day.AddDate(0, 0, -1)),
		DaysUntilExam:   int(day.Sub(today).Hours() / 24),
		Note:            "berechnet nach PVO §10/§11 (Klausur); maßgeblich ist, ob das CIS den Link anbietet",
	}
}

// FlashMessage extracts the TYPO3 flash/alert message of a response page.
func FlashMessage(body string) string {
	doc := htmlx.Main(htmlx.MustParse(body))
	for _, cls := range []string{"typo3-messages", "alert", "flash", "message"} {
		for _, n := range htmlx.All(doc, func(n *html.Node) bool {
			return n.Type == html.ElementNode && strings.Contains(htmlx.Attr(n, "class"), cls)
		}) {
			if t := htmlx.Text(n); t != "" {
				return t
			}
		}
	}
	return "Request sent (no confirmation message on the response page — check the CIS)."
}

func spanTexts(n *html.Node) []string {
	var out []string
	for _, s := range htmlx.All(n, htmlx.Tag("span")) {
		if t := htmlx.Text(s); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		if t := htmlx.Text(n); t != "" {
			out = append(out, t)
		}
	}
	return out
}
