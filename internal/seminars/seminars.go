// Package seminars reads the seminar programme (tx_naseminar) and performs
// seminar actions (sign-up, waitlist, …).
//
// Read actions (list, personalList, show, showParticipantList, showWaitList)
// are followed with Page; every other link on a row is treated as a binding
// action and only followed through client.WriteGet.
package seminars

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/forms"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

const PagePath = "/studium/bachelor/seminare"

var readActions = map[string]bool{"list": true, "personalList": true, "show": true, "showParticipantList": true, "showWaitList": true}

var categories = map[string]string{
	"na-filter-eth": "Ethik / Soziales", "na-filter-lang": "Internationales / Sprachen", "na-filter-meth": "Methodenkompetenz",
	"na-filter-pers": "Persönlichkeitsentwicklung", "na-filter-dkom": "Digitale Kompetenz", "na-filter-tech": "Technik",
}

// ActionNames translates known write actions.
var ActionNames = map[string]string{
	"subscribeToSeminar":   "anmelden",
	"anWartelisteAnmelden": "auf die Warteliste setzen",
}

type Action struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"-"`
}

type Seminar struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Lecturer      string   `json:"lecturer,omitempty"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Info          string   `json:"info,omitempty"` // Zeit / Anmerkung
	Category      string   `json:"category,omitempty"`
	Cancelled     bool     `json:"cancelled,omitempty"`
	Status        []string `json:"status,omitempty"` // e.g. "Seminar besucht", "Teilnehmer am Seminar"
	HasWaitlist   bool     `json:"has_waitlist,omitempty"`
	Actions       []Action `json:"actions,omitempty"` // binding actions currently offered
	DetailURL     string   `json:"-"`
	ParticipantsU string   `json:"-"`
	WaitlistURL   string   `json:"-"`
}

type Listing struct {
	Quarter    string            `json:"quarter"`
	Quarters   map[string]string `json:"available_quarters,omitempty"` // label -> id
	Notice     string            `json:"notice,omitempty"`
	Seminars   []Seminar         `json:"seminars"`
	PersonalOK bool              `json:"-"`
}

// Fetch lists seminars. quarter is a label like "2026 - 4" ("" = current);
// mine=true shows "Meine Seminare" (all quarters).
func Fetch(c *client.Client, quarter string, mine bool) (*Listing, error) {
	p, err := c.Page(PagePath)
	if err != nil {
		return nil, fmt.Errorf("seminare: %w", err)
	}
	l := Parse(p.Body, c.Base)
	if mine {
		doc := htmlx.MustParse(p.Body)
		a := htmlx.First(doc, htmlx.HrefContains("[action]=personalList"))
		if a == nil {
			return nil, fmt.Errorf("seminare: 'Meine Seminare' link not found")
		}
		p2, err := c.Page(htmlx.AbsURL(c.Base, htmlx.Attr(a, "href")))
		if err != nil {
			return nil, err
		}
		ml := Parse(p2.Body, c.Base)
		ml.Quarter = "Meine Seminare"
		return ml, nil
	}
	if quarter == "" || quarter == l.Quarter {
		return l, nil
	}
	id, ok := l.Quarters[quarter]
	if !ok {
		return nil, fmt.Errorf("unknown quarter %q", quarter)
	}
	f := forms.FindByField(forms.Parse(htmlx.MustParse(p.Body), c.Base), "quarterId")
	if f == nil {
		return nil, fmt.Errorf("quarter filter not found")
	}
	s, err := f.Submit("", map[string]string{"quarterId": id})
	if err != nil {
		return nil, err
	}
	p2, err := c.Page(s.QueryURL())
	if err != nil {
		return nil, err
	}
	return Parse(p2.Body, c.Base), nil
}

func Parse(body, base string) *Listing {
	doc := htmlx.MustParse(body)
	l := &Listing{Quarters: map[string]string{}}
	if sel := htmlx.First(doc, func(n *html.Node) bool {
		return htmlx.Tag("select")(n) && strings.HasSuffix(htmlx.Attr(n, "name"), "[quarterId]")
	}); sel != nil {
		for _, o := range htmlx.All(sel, htmlx.Tag("option")) {
			l.Quarters[htmlx.Text(o)] = htmlx.Attr(o, "value")
			if htmlx.HasAttr(o, "selected") {
				l.Quarter = htmlx.Text(o)
			}
		}
	}
	if pb := htmlx.First(doc, func(n *html.Node) bool {
		return htmlx.Class("panel-body")(n) && strings.Contains(htmlx.Text(n), "Wahlzeitraum")
	}); pb != nil {
		l.Notice = htmlx.Text(pb)
	}
	tbl := htmlx.First(doc, func(n *html.Node) bool {
		return htmlx.Tag("table")(n) && strings.HasPrefix(htmlx.Attr(n, "id"), "seminar-table")
	})
	if tbl == nil {
		return l
	}
	for _, tr := range htmlx.Rows(tbl) {
		if s, ok := parseRow(tr, base); ok {
			l.Seminars = append(l.Seminars, s)
		}
	}
	return l
}

func parseRow(tr *html.Node, base string) (Seminar, bool) {
	cells := htmlx.Children(tr, htmlx.Tag("td"))
	if len(cells) < 7 {
		return Seminar{}, false
	}
	var s Seminar
	if a := htmlx.First(cells[0], htmlx.HrefContains("[action]=show")); a != nil {
		s.DetailURL = htmlx.AbsURL(base, htmlx.Attr(a, "href"))
		s.ID = htmlx.QueryParam(s.DetailURL, "seminarId")
		s.Title = htmlx.Text(a)
	}
	if i := htmlx.First(cells[0], htmlx.Tag("i")); i != nil {
		s.Lecturer = htmlx.Text(i)
	}
	s.From, s.To = visibleDate(cells[1]), visibleDate(cells[2])
	s.Info = htmlx.LinesText(cells[3])
	for _, cls := range strings.Fields(htmlx.Attr(tr, "class")) {
		if c, ok := categories[cls]; ok {
			s.Category = c
		}
	}
	if htmlx.First(cells[0], htmlx.Class("glyphicon-ban-circle")) != nil {
		s.Cancelled = true
	}
	for _, g := range htmlx.All(cells[5], htmlx.Class("glyphicon")) {
		if t := htmlx.Attr(g, "title"); t != "" {
			s.Status = append(s.Status, t)
		}
	}
	for _, a := range htmlx.All(cells[6], htmlx.Tag("a")) {
		href := htmlx.AbsURL(base, htmlx.Attr(a, "href"))
		act := htmlx.Action(href)
		switch act {
		case "showParticipantList":
			s.ParticipantsU = href
		case "showWaitList":
			s.WaitlistURL, s.HasWaitlist = href, true
		case "", "show", "list", "personalList":
		default:
			label := ActionNames[act]
			if g := htmlx.First(a, htmlx.Class("glyphicon")); g != nil && htmlx.Attr(g, "title") != "" {
				label = htmlx.Attr(g, "title")
			}
			s.Actions = append(s.Actions, Action{Action: act, Label: label, URL: href})
		}
	}
	if s.ID == "" {
		return Seminar{}, false
	}
	return s, true
}

// visibleDate drops the hidden sort timestamp in front of the date.
func visibleDate(td *html.Node) string {
	t := htmlx.Text(td)
	if f := strings.Fields(t); len(f) > 1 && regexp.MustCompile(`^\d{9,}$`).MatchString(f[0]) {
		return strings.Join(f[1:], " ")
	}
	return t
}

// Find looks a seminar up by ID in the current list and in "Meine Seminare".
func Find(c *client.Client, id string) (*Seminar, error) {
	for _, mine := range []bool{false, true} {
		l, err := Fetch(c, "", mine)
		if err != nil {
			return nil, err
		}
		for i := range l.Seminars {
			if l.Seminars[i].ID == id {
				return &l.Seminars[i], nil
			}
		}
	}
	return nil, fmt.Errorf("seminar %s not found in the current programme or in 'Meine Seminare'", id)
}

// ── detail ──────────────────────────────────────────────────────────────────

type Detail struct {
	Seminar
	Remark      string `json:"remark,omitempty"`
	Topic       string `json:"themenbereich,omitempty"`
	ExamForm    string `json:"pruefungsform,omitempty"`
	Credits     string `json:"kreditpunkte,omitempty"`
	Workload    string `json:"workload,omitempty"`
	CV          string `json:"lecturer_cv,omitempty"`
	Description string `json:"description,omitempty"`
}

var detailLabels = []string{"Von", "Bis", "Bemerkung", "Themenbereich", "Prüfungsform", "Kreditpunkte", "Workload", "Dozent", "Lebenslauf", "Inhalt"}

func ParseDetail(body string) *Detail {
	doc := htmlx.Main(htmlx.MustParse(body))
	d := &Detail{}
	box := htmlx.First(doc, func(n *html.Node) bool {
		return htmlx.Class("col-md-12")(n) && htmlx.First(n, htmlx.Tag("h2")) != nil && strings.Contains(htmlx.Text(n), "Von:")
	})
	if box == nil {
		return d
	}
	d.Title = strings.TrimPrefix(htmlx.Text(htmlx.First(box, htmlx.Tag("h2"))), "Seminar ")
	fields := map[string]string{}
	var desc []string
	cur := ""
	for _, line := range strings.Split(htmlx.LinesText(box), "\n") {
		if line == htmlx.Text(htmlx.First(box, htmlx.Tag("h2"))) {
			continue
		}
		if l, rest, ok := labelLine(line); ok && (cur != "Inhalt" || l != "Inhalt") {
			cur = l
			if rest != "" {
				fields[l] = strings.TrimSpace(fields[l] + " " + rest)
			}
			continue
		}
		if cur == "Inhalt" {
			desc = append(desc, line)
		} else if cur != "" {
			fields[cur] = strings.TrimSpace(fields[cur] + "\n" + line)
		}
	}
	d.From, d.To, d.Remark = fields["Von"], fields["Bis"], fields["Bemerkung"]
	d.Topic, d.ExamForm, d.Credits, d.Workload = fields["Themenbereich"], fields["Prüfungsform"], fields["Kreditpunkte"], fields["Workload"]
	d.Lecturer, d.CV = fields["Dozent"], fields["Lebenslauf"]
	d.Description = strings.Join(desc, "\n")
	return d
}

func labelLine(line string) (string, string, bool) {
	for _, l := range detailLabels {
		for _, sep := range []string{l + ":", l + " :"} {
			if rest, ok := strings.CutPrefix(line, sep); ok {
				return l, strings.TrimSpace(rest), true
			}
		}
	}
	return "", "", false
}

func FetchDetail(c *client.Client, id string) (*Detail, error) {
	s, err := Find(c, id)
	if err != nil {
		return nil, err
	}
	p, err := c.Page(s.DetailURL)
	if err != nil {
		return nil, err
	}
	d := ParseDetail(p.Body)
	title := d.Title
	d.Seminar = *s
	if d.Title == "" {
		d.Title = title
	}
	return d, nil
}

// ── participants (count only) ───────────────────────────────────────────────

// Participation deliberately reports counts only — participant lists contain
// other students' names (DSGVO).
type Participation struct {
	SeminarID    string `json:"seminar_id"`
	Participants int    `json:"participants"`
	Waitlist     int    `json:"waitlist,omitempty"`
	YouListed    bool   `json:"you_are_listed"`
	YourPosition int    `json:"your_position,omitempty"`
	MinRequired  int    `json:"min_required"`
}

// CountList counts rows of a Teilnehmer-/Warteliste and finds the student.
func CountList(body, nachname, vorname string) (n int, pos int) {
	doc := htmlx.Main(htmlx.MustParse(body))
	for _, tbl := range htmlx.All(doc, htmlx.Tag("table")) {
		if !strings.Contains(htmlx.Text(htmlx.First(tbl, htmlx.Tag("tr"))), "Nachname") {
			continue
		}
		for _, tr := range htmlx.Rows(tbl) {
			cells := htmlx.Cells(tr)
			if len(cells) < 3 {
				continue
			}
			n++
			if pos == 0 && nachname != "" && strings.EqualFold(htmlx.Text(cells[1]), nachname) && strings.EqualFold(htmlx.Text(cells[2]), vorname) {
				pos = n
			}
		}
	}
	return n, pos
}

func FetchParticipation(c *client.Client, id, nachname, vorname string) (*Participation, error) {
	s, err := Find(c, id)
	if err != nil {
		return nil, err
	}
	pt := &Participation{SeminarID: id, MinRequired: 16}
	if s.ParticipantsU != "" {
		p, err := c.Page(s.ParticipantsU)
		if err != nil {
			return nil, err
		}
		var pos int
		pt.Participants, pos = CountList(p.Body, nachname, vorname)
		pt.YouListed = pos > 0
	}
	if s.WaitlistURL != "" {
		p, err := c.Page(s.WaitlistURL)
		if err != nil {
			return nil, err
		}
		var pos int
		pt.Waitlist, pos = CountList(p.Body, nachname, vorname)
		if pos > 0 {
			pt.YouListed, pt.YourPosition = true, pos
		}
	}
	return pt, nil
}

// ── binding actions ─────────────────────────────────────────────────────────

// ResolveAction returns the live action link of a seminar. action may be the
// TYPO3 name (subscribeToSeminar) or "" when exactly one action is offered.
func ResolveAction(c *client.Client, id, action string) (*Seminar, *Action, error) {
	s, err := Find(c, id)
	if err != nil {
		return nil, nil, err
	}
	// The waitlist sign-up button lives on the waitlist page, not in the table.
	if s.WaitlistURL != "" {
		if p, err := c.Page(s.WaitlistURL); err == nil {
			s.Actions = append(s.Actions, PageActions(p.Body, c.Base, s.ID)...)
		}
	}
	if len(s.Actions) == 0 {
		return s, nil, fmt.Errorf("seminar %s (%s) currently offers no action", id, s.Title)
	}
	if action == "" {
		if len(s.Actions) > 1 {
			return s, nil, fmt.Errorf("seminar %s offers several actions, choose one: %s", id, actionList(s.Actions))
		}
		return s, &s.Actions[0], nil
	}
	for i := range s.Actions {
		if s.Actions[i].Action == action {
			return s, &s.Actions[i], nil
		}
	}
	return s, nil, fmt.Errorf("seminar %s does not offer %q; offered: %s", id, action, actionList(s.Actions))
}

func actionList(as []Action) string {
	var p []string
	for _, a := range as {
		p = append(p, fmt.Sprintf("%s (%s)", a.Action, a.Label))
	}
	return strings.Join(p, ", ")
}

// PageActions returns write-action links for seminarID found anywhere on a page.
func PageActions(body, base, seminarID string) []Action {
	var out []Action
	for _, a := range htmlx.All(htmlx.Main(htmlx.MustParse(body)), htmlx.Tag("a")) {
		href := htmlx.AbsURL(base, htmlx.Attr(a, "href"))
		act := htmlx.Action(href)
		if act == "" || readActions[act] || htmlx.QueryParam(href, "seminarId") != seminarID {
			continue
		}
		label := htmlx.Text(a)
		if label == "" {
			label = ActionNames[act]
		}
		out = append(out, Action{Action: act, Label: label, URL: href})
	}
	return out
}

// Submit follows the action link. BINDING.
func Submit(c *client.Client, a *Action) (*client.Page, error) {
	if readActions[a.Action] {
		return nil, fmt.Errorf("%s is not a write action", a.Action)
	}
	return c.WriteGet(a.URL)
}
