// Package stundenplan reads the Bachelor timetable overview from the CIS.
//
// The CIS does not expose a structured timetable. Instead the page
// /studium/bachelor/stundenplaene offers, per Zenturie (class group), a set of
// downloadable files — an .ics calendar and an .html overview — rendered as a
// TYPO3 "ce-uploads" list. This package lists those files and downloads them.
package stundenplan

import (
	"fmt"
	"io"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"golang.org/x/net/html"
)

// PagePath is the timetable overview page.
const PagePath = "/studium/bachelor/stundenplaene"

// Plan is a single downloadable timetable file for one Zenturie.
type Plan struct {
	Zenturie string `json:"zenturie"` // file name without extension, e.g. "A24a_4"
	Filename string `json:"filename"` // e.g. "A24a_4.ics"
	Format   string `json:"format"`   // "ics", "html", ...
	URL      string `json:"url"`      // absolute download URL
	Date     string `json:"date"`     // upload date as shown, e.g. "13.05.2026"
	Size     string `json:"size"`     // human-readable size, e.g. "126 KB"
}

// FetchList fetches the timetable overview and returns all downloadable plans.
func FetchList(c *client.Client) ([]Plan, error) {
	resp, err := c.Get(PagePath)
	if err != nil {
		return nil, fmt.Errorf("fetch stundenplaene: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parsePlans(string(body)), nil
}

// Download fetches a plan file and returns its raw bytes plus content type.
func Download(c *client.Client, downloadURL string) ([]byte, string, error) {
	resp, err := c.GetURL(downloadURL)
	if err != nil {
		return nil, "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ct, fmt.Errorf("read download: %w", err)
	}
	return data, ct, nil
}

// Filter returns plans whose Zenturie starts with the given prefix
// (case-insensitive). An empty prefix returns the input unchanged.
func Filter(plans []Plan, zenturiePrefix, format string) []Plan {
	if zenturiePrefix == "" && format == "" {
		return plans
	}
	pfx := strings.ToLower(zenturiePrefix)
	out := make([]Plan, 0, len(plans))
	for _, p := range plans {
		if pfx != "" && !strings.HasPrefix(strings.ToLower(p.Zenturie), pfx) {
			continue
		}
		if format != "" && !strings.EqualFold(p.Format, format) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// parsePlans extracts every <li> inside a <ul class="ce-uploads"> list.
func parsePlans(body string) []Plan {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	var plans []Plan
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "ul" && strings.Contains(attr(n, "class"), "ce-uploads") {
			for li := n.FirstChild; li != nil; li = li.NextSibling {
				if li.Type == html.ElementNode && li.Data == "li" {
					if p, ok := parseUploadItem(li); ok {
						plans = append(plans, p)
					}
				}
			}
			return // a ce-uploads list does not nest another
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return plans
}

func parseUploadItem(li *html.Node) (Plan, bool) {
	var p Plan
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch {
			case n.Data == "a":
				if href := attr(n, "href"); href != "" && p.URL == "" {
					p.URL = normalizeURL(href)
				}
			case n.Data == "span":
				switch class := attr(n, "class"); {
				case strings.Contains(class, "ce-uploads-fileName"):
					p.Filename = strings.TrimSpace(innerText(n))
				case strings.Contains(class, "ce-uploads-time"):
					p.Date = strings.TrimSpace(innerText(n))
				case strings.Contains(class, "ce-uploads-filesize"):
					p.Size = strings.TrimSpace(innerText(n))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(li)

	if p.Filename == "" || p.URL == "" {
		return Plan{}, false
	}
	if dot := strings.LastIndex(p.Filename, "."); dot >= 0 {
		p.Zenturie = p.Filename[:dot]
		p.Format = strings.ToLower(p.Filename[dot+1:])
	} else {
		p.Zenturie = p.Filename
	}
	return p, true
}

// normalizeURL turns a possibly relative, HTML-entity-encoded href into an
// absolute URL. TYPO3 emits dumpFile links with &amp; separators.
func normalizeURL(href string) string {
	href = strings.ReplaceAll(href, "&amp;", "&")
	if !strings.HasPrefix(href, "http") {
		href = client.BaseURL + href
	}
	return href
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
