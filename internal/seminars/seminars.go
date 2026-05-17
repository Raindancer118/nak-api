package seminars

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/scraper"
	"golang.org/x/net/html"
)

type Seminar struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	HasWait   bool   `json:"waitlist_only"`
	DetailURL string `json:"detail_url"`
}

type SeminarDetail struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	HasWait     bool     `json:"waitlist_only"`
	Description string   `json:"description"`
	Dozenten    []string `json:"dozenten"`
	Credits     string   `json:"credits"`
	Pruefung    string   `json:"pruefung"`
	Termine     string   `json:"termine"`
	Methoden    string   `json:"lehr_methoden"`
}

// FetchList returns all seminars from the overview page.
func FetchList(c *client.Client) ([]Seminar, error) {
	resp, err := c.Get("/studium/bachelor/seminare")
	if err != nil {
		return nil, fmt.Errorf("fetch seminare: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseSeminarList(string(body)), nil
}

func parseSeminarList(body string) []Seminar {
	doc, _ := html.Parse(strings.NewReader(body))

	// Collect all seminar IDs that have a waitlist link
	waitIDs := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			if strings.Contains(href, "showWaitList") {
				id := extractParam(href, "seminarId")
				if id != "" {
					waitIDs[id] = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Collect show links with titles (deduplicate by ID)
	seen := map[string]bool{}
	var seminars []Seminar
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			if strings.Contains(href, "action%5D=show") || strings.Contains(href, "action]=show") {
				if !strings.Contains(href, "showWait") && !strings.Contains(href, "showParticipant") {
					id := extractParam(href, "seminarId")
					title := strings.TrimSpace(innerText(n))
					if id != "" && title != "" && !seen[id] {
						seen[id] = true
						absURL := href
						if !strings.HasPrefix(absURL, "http") {
							absURL = client.BaseURL + absURL
						}
						seminars = append(seminars, Seminar{
							ID:        id,
							Title:     title,
							HasWait:   waitIDs[id],
							DetailURL: absURL,
						})
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return seminars
}

// FetchDetail fetches the detail page for a seminar and returns structured info.
func FetchDetail(c *client.Client, seminarID string) (*SeminarDetail, error) {
	// First get the list page to find the proper cHash'd URL for this ID
	seminars, err := FetchList(c)
	if err != nil {
		return nil, err
	}
	var detailURL string
	for _, s := range seminars {
		if s.ID == seminarID {
			detailURL = s.DetailURL
			break
		}
	}
	if detailURL == "" {
		return nil, fmt.Errorf("seminar id %s not found", seminarID)
	}

	resp, err := c.GetURL(detailURL)
	if err != nil {
		return nil, fmt.Errorf("fetch seminar detail: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseSeminarDetail(seminarID, string(body)), nil
}

func parseSeminarDetail(id, body string) *SeminarDetail {
	tables, _ := scraper.ExtractTables(strings.NewReader(body))

	d := &SeminarDetail{ID: id}

	// Extract h1/h2 for title
	doc, _ := html.Parse(strings.NewReader(body))
	var walkH func(*html.Node)
	walkH = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "h1" || n.Data == "h2") {
			t := strings.TrimSpace(innerText(n))
			if t != "" && !strings.Contains(t, "NORDAKADEMIE") && !strings.Contains(t, "Social Media") && d.Title == "" {
				d.Title = t
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkH(c)
		}
	}
	walkH(doc)

	// Extract labeled fields from tables
	for _, t := range tables {
		for _, row := range t.Rows {
			if len(row) < 2 {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(row[0]))
			val := strings.TrimSpace(row[1])
			switch {
			case strings.Contains(key, "dozent"):
				d.Dozenten = append(d.Dozenten, val)
			case strings.Contains(key, "credit") || strings.Contains(key, "cp"):
				d.Credits = val
			case strings.Contains(key, "prüfung"):
				d.Pruefung = val
			case strings.Contains(key, "termin"):
				d.Termine = val
			case strings.Contains(key, "lehr") || strings.Contains(key, "methode"):
				d.Methoden = val
			case strings.Contains(key, "lernziel") || strings.Contains(key, "beschreibung") || strings.Contains(key, "inhalt"):
				d.Description = val
			}
		}
	}

	// Fallback: extract labeled paragraphs
	if d.Credits == "" || d.Pruefung == "" {
		parseLabeledParagraphs(body, d)
	}
	return d
}

func parseLabeledParagraphs(body string, d *SeminarDetail) {
	doc, _ := html.Parse(strings.NewReader(body))
	var prev string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "dt", "th", "strong", "b":
				prev = strings.ToLower(strings.TrimSpace(innerText(n)))
			case "dd", "td", "p":
				val := strings.TrimSpace(innerText(n))
				if val == "" || len(val) > 500 {
					prev = ""
					break
				}
				switch {
				case strings.Contains(prev, "dozent"):
					if !contains(d.Dozenten, val) {
						d.Dozenten = append(d.Dozenten, val)
					}
				case strings.Contains(prev, "credit") || prev == "cp":
					if d.Credits == "" {
						d.Credits = val
					}
				case strings.Contains(prev, "prüfung"):
					if d.Pruefung == "" {
						d.Pruefung = val
					}
				case strings.Contains(prev, "termin"):
					if d.Termine == "" {
						d.Termine = val
					}
				case strings.Contains(prev, "lernziel") || strings.Contains(prev, "inhalt"):
					if d.Description == "" {
						d.Description = val
					}
				}
				prev = ""
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

func extractParam(rawURL, param string) string {
	decoded, _ := url.QueryUnescape(rawURL)
	u, err := url.Parse(decoded)
	if err != nil {
		return ""
	}
	for k, v := range u.Query() {
		// TYPO3 uses tx_ext_plugin[param] notation
		if strings.HasSuffix(k, "["+param+"]") || k == param {
			if len(v) > 0 {
				return v[0]
			}
		}
	}
	return ""
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

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
