// Package wahlpflicht reads the elective catalogue (tx_nawahlpflichtmodule)
// and prepares the binding module choice.
//
// Flow observed in the CIS: the list renders one POST form per module
// (action=show, fields id + curriculumId + __trustedProperties) leading to the
// detail view. During the selection period the detail view lists the
// Termine with a cHash-signed GET link action=order&id=<Termin-ID> — following
// that link IS the binding choice. Outside the period the list is empty.
package wahlpflicht

import (
	"fmt"
	"strings"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/forms"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

const PagePath = "/studium/bachelor/wahlpflichtkurse/wahlpflichtkurse-waehlen"

type Module struct {
	ID           string `json:"id"`
	CurriculumID string `json:"curriculum_id"`
	Name         string `json:"name"`
	Dozent       string `json:"dozent,omitempty"`
	Vertiefung   string `json:"vertiefungsrichtung,omitempty"`
	Termin       string `json:"termin,omitempty"` // weekday / remark column
	Extra        string `json:"extra,omitempty"`
	Chosen       bool   `json:"chosen"`
}

type List struct {
	Chosen    []Module `json:"chosen"`
	Available []Module `json:"available"`
	Open      bool     `json:"selection_open"`
	Notice    string   `json:"notice,omitempty"`
}

func Fetch(c *client.Client) (*List, error) {
	p, err := c.Page(PagePath)
	if err != nil {
		return nil, fmt.Errorf("wahlpflicht: %w", err)
	}
	return Parse(p.Body), nil
}

// FetchModules is the flat list (chosen first) used by older callers.
func FetchModules(c *client.Client) ([]Module, error) {
	l, err := Fetch(c)
	if err != nil {
		return nil, err
	}
	return append(append([]Module{}, l.Chosen...), l.Available...), nil
}

func Parse(body string) *List {
	doc := htmlx.Main(htmlx.MustParse(body))
	l := &List{}
	for _, h := range htmlx.All(doc, htmlx.Tag("h3")) {
		title := strings.ToLower(htmlx.Text(h))
		tbl := nextTable(h)
		switch {
		case strings.HasPrefix(title, "gewählte module"):
			if tbl != nil {
				l.Chosen = parseRows(tbl, true)
			}
		case strings.HasPrefix(title, "alle wählbaren module"):
			if tbl != nil {
				l.Available = parseRows(tbl, false)
			}
			if t := htmlx.Text(h); strings.Contains(t, "Aktuell stehen") {
				l.Notice = strings.TrimSpace(t[strings.Index(t, "Aktuell"):])
			}
		}
	}
	if l.Notice == "" {
		if n := htmlx.First(doc, func(n *html.Node) bool {
			return n.Type == html.TextNode && strings.Contains(n.Data, "Aktuell stehen noch keine")
		}); n != nil {
			l.Notice = htmlx.Collapse(n.Data)
		}
	}
	l.Open = len(l.Available) > 0
	return l
}

// nextTable returns the first <table> after h in document order within its parent chain.
func nextTable(h *html.Node) *html.Node {
	for n := h; n != nil; n = n.Parent {
		for s := n.NextSibling; s != nil; s = s.NextSibling {
			if htmlx.Tag("h3")(s) {
				return nil
			}
			if t := htmlx.First(s, htmlx.Tag("table")); t != nil {
				return t
			}
		}
	}
	return nil
}

func parseRows(tbl *html.Node, chosen bool) []Module {
	var out []Module
	for _, tr := range htmlx.Rows(tbl) {
		cells := htmlx.Children(tr, htmlx.Tag("td"))
		if len(cells) < 2 {
			continue
		}
		m := Module{Chosen: chosen}
		for _, in := range htmlx.All(cells[0], htmlx.Tag("input")) {
			switch name := htmlx.Attr(in, "name"); {
			case strings.HasSuffix(name, "[id]"):
				m.ID = htmlx.Attr(in, "value")
			case strings.HasSuffix(name, "[curriculumId]"):
				m.CurriculumID = htmlx.Attr(in, "value")
			}
		}
		texts := make([]string, 0, len(cells)-1)
		for _, c := range cells[1:] {
			texts = append(texts, htmlx.Text(c))
		}
		if chosen {
			m.Name = at(texts, 0)
			m.Termin = at(texts, 1)
		} else {
			m.Name, m.Dozent, m.Vertiefung, m.Termin, m.Extra = at(texts, 0), at(texts, 1), at(texts, 2), at(texts, 3), at(texts, 4)
		}
		if m.ID != "" {
			out = append(out, m)
		}
	}
	return out
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// ── detail ──────────────────────────────────────────────────────────────────

type Termin struct {
	ID       string `json:"id,omitempty"` // Termin-ID from the order link
	Text     string `json:"text"`
	OrderURL string `json:"-"`
}

type Detail struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Lerninhalte string   `json:"lerninhalte,omitempty"`
	Methoden    string   `json:"lehr_und_lernmethoden,omitempty"`
	Lernziele   string   `json:"lernziele,omitempty"`
	Literatur   string   `json:"literatur,omitempty"`
	Pruefung    string   `json:"pruefungsform,omitempty"`
	Dozenten    []string `json:"dozenten,omitempty"`
	Credits     string   `json:"creditpoints,omitempty"`
	Workload    string   `json:"workload,omitempty"`
	Termine     []Termin `json:"termine"`
	SelectAvail bool     `json:"select_available"`
}

func ParseDetail(id, body, base string) *Detail {
	doc := htmlx.Main(htmlx.MustParse(body))
	d := &Detail{ID: id}
	box := htmlx.First(doc, htmlx.Class("tx-na-wahlpflichtmodul"))
	if box == nil {
		return d
	}
	d.Title = htmlx.Text(htmlx.First(box, htmlx.Tag("h2")))
	// Not Children: a stray "<div />" inside the Dozent paragraph re-parents the following <p>s.
	for _, p := range htmlx.All(box, htmlx.Tag("p")) {
		b := htmlx.First(p, htmlx.Tag("b", "strong"))
		if b == nil {
			continue
		}
		label := strings.TrimSuffix(htmlx.Text(b), ":")
		full := htmlx.LinesText(p)
		val := strings.TrimSpace(strings.TrimPrefix(full, htmlx.Text(b)))
		switch {
		case label == "Lerninhalte":
			d.Lerninhalte = val
		case strings.HasPrefix(label, "Lehr- und Lernmethoden"):
			d.Methoden = val
		case label == "Lernziele":
			d.Lernziele = val
		case label == "Literatur":
			d.Literatur = val
		case label == "Prüfungsform":
			d.Pruefung = val
		case label == "Creditpoints":
			d.Credits = val
		case strings.HasPrefix(label, "Workload"):
			d.Workload = label
		}
	}
	for _, dz := range htmlx.All(box, htmlx.Class("weiterbildungsmodul_dozent")) {
		if t := htmlx.Text(dz); t != "" {
			d.Dozenten = append(d.Dozenten, strings.Join(strings.Fields(t), " "))
		}
	}
	for _, ul := range htmlx.All(box, htmlx.Class("list-group")) {
		for _, li := range htmlx.All(ul, htmlx.Tag("li")) {
			t := Termin{Text: htmlx.Text(li)}
			if a := htmlx.First(li, htmlx.HrefContains("[action]=order")); a != nil {
				t.OrderURL = htmlx.AbsURL(base, htmlx.Attr(a, "href"))
				t.ID = htmlx.QueryParam(t.OrderURL, "id")
				t.Text = strings.TrimSpace(strings.TrimSuffix(t.Text, htmlx.Text(a)))
				d.SelectAvail = true
			}
			d.Termine = append(d.Termine, t)
		}
	}
	return d
}

// showForm finds the module's detail form on the list page.
func showForm(body, base, moduleID string) (*forms.Form, error) {
	for _, f := range forms.Parse(htmlx.Main(htmlx.MustParse(body)), base) {
		if !strings.Contains(f.Action, "action%5D=show") && !strings.Contains(f.Action, "action]=show") {
			continue
		}
		if fl := f.Field("id"); fl != nil && fl.Value == moduleID {
			return f, nil
		}
	}
	return nil, fmt.Errorf("Wahlpflichtmodul %s is not listed (neither chosen nor selectable)", moduleID)
}

// FetchDetail opens the detail view. The show form only displays data.
func FetchDetail(c *client.Client, moduleID string) (*Detail, error) {
	p, err := c.Page(PagePath)
	if err != nil {
		return nil, err
	}
	f, err := showForm(p.Body, c.Base, moduleID)
	if err != nil {
		return nil, err
	}
	s, err := f.Submit("", nil)
	if err != nil {
		return nil, err
	}
	p2, err := c.PostRead(s.URL, s.Values())
	if err != nil {
		return nil, err
	}
	return ParseDetail(moduleID, p2.Body, c.Base), nil
}

// Choice is a prepared, not yet sent, module selection.
type Choice struct {
	Module string `json:"module"`
	Termin Termin `json:"termin"`
}

// PrepareSelect resolves the order link. terminID may be "" when the module
// offers exactly one selectable Termin.
func PrepareSelect(c *client.Client, moduleID, terminID string) (*Choice, error) {
	d, err := FetchDetail(c, moduleID)
	if err != nil {
		return nil, err
	}
	return choose(d, terminID)
}

func choose(d *Detail, terminID string) (*Choice, error) {
	var orderable []Termin
	for _, t := range d.Termine {
		if t.OrderURL != "" {
			orderable = append(orderable, t)
		}
	}
	if len(orderable) == 0 {
		return nil, fmt.Errorf("%s: no selectable Termin — the selection period is not open", d.Title)
	}
	if terminID == "" {
		if len(orderable) > 1 {
			var ids []string
			for _, t := range orderable {
				ids = append(ids, t.ID+" ("+t.Text+")")
			}
			return nil, fmt.Errorf("%s has several Termine, pass termin_id: %s", d.Title, strings.Join(ids, ", "))
		}
		return &Choice{Module: d.Title, Termin: orderable[0]}, nil
	}
	for _, t := range orderable {
		if t.ID == terminID {
			return &Choice{Module: d.Title, Termin: t}, nil
		}
	}
	return nil, fmt.Errorf("%s has no selectable Termin %s", d.Title, terminID)
}

// Submit follows the order link. BINDING.
func Submit(c *client.Client, ch *Choice) (*client.Page, error) {
	return c.WriteGet(ch.Termin.OrderURL)
}
