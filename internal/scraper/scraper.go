package scraper

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// FormFields extracts all hidden input values and the form action from the
// first form matching the given action substring.
type FormFields struct {
	Action string
	Fields map[string]string
}

func ExtractForm(r io.Reader, actionContains string) (*FormFields, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var form *FormFields
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if form != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "form" {
			action := attr(n, "action")
			if strings.Contains(action, actionContains) {
				form = &FormFields{Action: action, Fields: make(map[string]string)}
				extractInputs(n, form.Fields)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return form, nil
}

func extractInputs(n *html.Node, fields map[string]string) {
	if n.Type == html.ElementNode && n.Data == "input" {
		name := attr(n, "name")
		value := attr(n, "value")
		if name != "" {
			fields[name] = value
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractInputs(c, fields)
	}
}

// ExtractLinks returns all href attributes of <a> tags whose href contains
// the given substring.
func ExtractLinks(r io.Reader, hrefContains string) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var links []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			if strings.Contains(href, hrefContains) {
				links = append(links, href)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return links, nil
}

// Table holds the parsed content of an HTML table.
type Table struct {
	Headers []string
	Rows    [][]string
}

// ExtractTables parses all <table> elements from the document.
func ExtractTables(r io.Reader) ([]Table, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var tables []Table
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			tables = append(tables, parseTable(n))
			return // don't recurse into nested tables
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return tables, nil
}

func parseTable(n *html.Node) Table {
	var t Table
	var walkRows func(*html.Node)
	walkRows = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for td := n.FirstChild; td != nil; td = td.NextSibling {
				if td.Type == html.ElementNode && (td.Data == "td" || td.Data == "th") {
					text := strings.TrimSpace(innerText(td))
					cells = append(cells, text)
				}
			}
			if len(cells) > 0 {
				if isHeaderRow(n) {
					t.Headers = cells
				} else {
					t.Rows = append(t.Rows, cells)
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkRows(c)
		}
	}
	walkRows(n)
	return t
}

func isHeaderRow(tr *html.Node) bool {
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "th" {
			return true
		}
	}
	// Also check if the tr is inside a <thead>
	if tr.Parent != nil && tr.Parent.Data == "thead" {
		return true
	}
	return false
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

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// ExtractMeta returns the content of the first meta tag with the given name.
func ExtractMeta(r io.Reader, name string) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	var result string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			if attr(n, "name") == name {
				result = attr(n, "content")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return result, nil
}
