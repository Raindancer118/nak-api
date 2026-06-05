// Package exams reads the exam registration overview (Prüfungsan-/abmeldung)
// from the CIS and performs register/deregister actions.
//
// The page /studium/pruefungen/an-und-abmeldung-von-pruefungen renders a table
// (TYPO3 extension tx_naexams, controller Pruefungsverwaltung). Each row offers
// at most one action link in the "An-/Abmelden" column — "register" when you may
// sign up, or a deregister variant when you are already registered. The action,
// its label and its cHash-signed URL are read directly from the page, so this
// package never has to construct a cHash itself.
package exams

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"golang.org/x/net/html"
)

// PagePath is the exam registration overview.
const PagePath = "/studium/pruefungen/an-und-abmeldung-von-pruefungen"

// Exam is one row of the Prüfungsübersicht.
type Exam struct {
	ExamID      string   `json:"exam_id"`
	ModuleNr    string   `json:"module_nr"`
	Title       string   `json:"title"`
	Zenturien   []string `json:"zenturien"`
	Dozenten    []string `json:"dozenten"`
	Start       string   `json:"start"`
	Ende        string   `json:"ende"`
	Status      string   `json:"status"`
	Action      string   `json:"action"`       // "register", "deregister", ... or "" if none offered
	ActionLabel string   `json:"action_label"` // visible button text, e.g. "Anmelden"
	ActionURL   string   `json:"action_url"`   // absolute, cHash-signed; "" if no action
}

// FetchList fetches and parses the exam overview.
func FetchList(c *client.Client) ([]Exam, error) {
	resp, err := c.Get(PagePath)
	if err != nil {
		return nil, fmt.Errorf("fetch exams: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseExams(string(body)), nil
}

// Resolve fetches the live overview and returns the exam with the given ID,
// including its current action link. Returns an error if the exam is not listed
// or currently offers no action.
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

// Submit performs the register/deregister action by following the page-provided,
// cHash-signed URL. This is a binding write operation — call it only after an
// explicit user confirmation. It returns a short result message.
func Submit(c *client.Client, actionURL string) (string, error) {
	resp, err := c.GetURL(actionURL)
	if err != nil {
		return "", fmt.Errorf("submit exam action: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return extractFlashMessage(string(body)), nil
}

func parseExams(body string) []Exam {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	table := findExamTable(doc)
	if table == nil {
		return nil
	}

	var exams []Exam
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			if e, ok := parseRow(n); ok {
				exams = append(exams, e)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	// Only walk the tbody so the thead header row is skipped.
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "tbody" {
			walk(c)
		}
	}
	return exams
}

// findExamTable returns the first table whose header contains both a
// "Modulnummer" and an "An-/Abmelden" column — i.e. the Prüfungsübersicht and
// not some unrelated table on the page.
func findExamTable(doc *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "table" {
			head := strings.ToLower(innerText(n))
			if strings.Contains(head, "modulnummer") && strings.Contains(head, "abmelden") {
				found = n
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func parseRow(tr *html.Node) (Exam, bool) {
	var cells []*html.Node
	for td := tr.FirstChild; td != nil; td = td.NextSibling {
		if td.Type == html.ElementNode && td.Data == "td" {
			cells = append(cells, td)
		}
	}
	if len(cells) < 8 {
		return Exam{}, false
	}

	e := Exam{
		ModuleNr:  textOf(cells[0]),
		Title:     textOf(cells[1]),
		Zenturien: spanTexts(cells[2]),
		Dozenten:  spanTexts(cells[3]),
		Start:     textOf(cells[4]),
		Ende:      textOf(cells[5]),
		Status:    textOf(cells[6]),
	}

	// Action cell: find the first <a href> and read its action + examId + label.
	if a := findFirstAnchor(cells[7]); a != nil {
		href := normalizeURL(attr(a, "href"))
		e.ActionURL = href
		e.ActionLabel = textOf(a)
		e.Action = queryParam(href, "action")
		e.ExamID = queryParam(href, "examId")
	}
	if e.ModuleNr == "" && e.Title == "" {
		return Exam{}, false
	}
	return e, true
}

func extractFlashMessage(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "Action submitted."
	}
	var msg string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if msg != "" {
			return
		}
		if n.Type == html.ElementNode {
			class := attr(n, "class")
			if strings.Contains(class, "alert") || strings.Contains(class, "message") || strings.Contains(class, "flash") {
				if t := strings.TrimSpace(innerText(n)); t != "" {
					msg = t
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if msg == "" {
		return "Action submitted (no confirmation message found on the response page)."
	}
	return msg
}

// ── small HTML helpers ──────────────────────────────────────────────────────

func normalizeURL(href string) string {
	href = strings.ReplaceAll(href, "&amp;", "&")
	if href != "" && !strings.HasPrefix(href, "http") {
		href = client.BaseURL + href
	}
	return href
}

// queryParam returns a TYPO3-style query parameter. It matches both the bare
// name and the tx_ext[name] bracket notation.
func queryParam(rawURL, param string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for k, v := range u.Query() {
		if (k == param || strings.HasSuffix(k, "["+param+"]")) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func findFirstAnchor(n *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" && attr(n, "href") != "" {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

func spanTexts(n *html.Node) []string {
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "span" {
			if t := strings.TrimSpace(innerText(n)); t != "" {
				out = append(out, t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if len(out) == 0 {
		if t := textOf(n); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func textOf(n *html.Node) string {
	return strings.TrimSpace(collapseSpace(innerText(n)))
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func innerText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}
