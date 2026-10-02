// Package pages reads arbitrary CIS information pages as text plus their
// downloadable documents and sub-pages. Only plain pages and links with an
// explicitly read-only TYPO3 action can be opened.
package pages

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

var readActions = map[string]bool{"show": true, "list": true}

type Link struct {
	Text    string `json:"text"`
	URL     string `json:"url"`
	Section string `json:"section,omitempty"`
}

type Page struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Downloads []Link `json:"downloads,omitempty"`
	SubPages  []Link `json:"sub_pages,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Allowed reports whether pathOrURL may be opened by the generic reader.
func Allowed(pathOrURL string) error {
	u, err := url.Parse(strings.ReplaceAll(pathOrURL, "&amp;", "&"))
	if err != nil {
		return err
	}
	if strings.Contains(u.Path, "logout") || u.Query().Get("logintype") != "" {
		return fmt.Errorf("refusing logout links")
	}
	if u.RawQuery == "" {
		return nil
	}
	if act := htmlx.Action(u.String()); act != "" && readActions[act] {
		return nil
	}
	return fmt.Errorf("only plain pages (no query) or read-only show/list links can be opened generically")
}

func Fetch(c *client.Client, pathOrURL string, maxChars int) (*Page, error) {
	if err := Allowed(pathOrURL); err != nil {
		return nil, err
	}
	p, err := c.Page(pathOrURL)
	if err != nil {
		return nil, err
	}
	if p.Status == 404 {
		return nil, fmt.Errorf("page not found: %s", pathOrURL)
	}
	pg := Parse(p.Body, c.Base)
	pg.URL = p.URL
	if maxChars > 0 && len([]rune(pg.Text)) > maxChars {
		pg.Text = string([]rune(pg.Text)[:maxChars])
		pg.Truncated = true
	}
	return pg, nil
}

func Parse(body, base string) *Page {
	doc := htmlx.MustParse(body)
	main := htmlx.Main(doc)
	pg := &Page{}
	if h := htmlx.First(main, htmlx.Tag("h1")); h != nil {
		pg.Title = htmlx.Text(h)
	} else if t := htmlx.First(doc, htmlx.Tag("title")); t != nil {
		pg.Title = htmlx.Text(t)
	}
	content := main
	if bc := htmlx.First(main, htmlx.Class("breadcrumb")); bc != nil {
		bc.Parent.RemoveChild(bc)
	}
	pg.Text = htmlx.LinesText(content)
	seen := map[string]bool{}
	section := ""
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if htmlx.Tag("h2", "h3", "h4")(n) {
			section = htmlx.Text(n)
		}
		if htmlx.Tag("a")(n) {
			href := htmlx.Attr(n, "href")
			if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "mailto:") && !strings.HasPrefix(href, "tel:") {
				abs := htmlx.AbsURL(base, href)
				text := htmlx.Text(n)
				if !seen[abs] && text != "" {
					seen[abs] = true
					l := Link{Text: text, URL: abs, Section: section}
					switch {
					case strings.Contains(abs, "eID=dumpFile") || strings.Contains(abs, "/fileadmin/") || strings.HasSuffix(strings.ToLower(abs), ".pdf"):
						pg.Downloads = append(pg.Downloads, l)
					case strings.HasPrefix(abs, base) && Allowed(abs) == nil:
						pg.SubPages = append(pg.SubPages, l)
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(content)
	return pg
}
