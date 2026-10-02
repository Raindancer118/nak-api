// Package htmlx holds the small HTML helpers every CIS scraper needs: node
// matching, text extraction and TYPO3 link handling.
package htmlx

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Match decides whether a node is wanted.
type Match func(*html.Node) bool

func Parse(body string) (*html.Node, error) { return html.Parse(strings.NewReader(body)) }

// MustParse never fails in practice: x/net/html recovers from any malformed input.
func MustParse(body string) *html.Node {
	n, err := Parse(body)
	if err != nil {
		return &html.Node{Type: html.DocumentNode}
	}
	return n
}

func Tag(names ...string) Match {
	return func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return false
		}
		for _, t := range names {
			if n.Data == t {
				return true
			}
		}
		return false
	}
}

func Class(c string) Match {
	return func(n *html.Node) bool { return n.Type == html.ElementNode && HasClass(n, c) }
}

func ID(id string) Match {
	return func(n *html.Node) bool { return n.Type == html.ElementNode && Attr(n, "id") == id }
}

func And(ms ...Match) Match {
	return func(n *html.Node) bool {
		for _, m := range ms {
			if !m(n) {
				return false
			}
		}
		return true
	}
}

// HrefContains matches <a> elements whose (entity-decoded, unescaped) href contains s.
func HrefContains(s string) Match {
	return func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return false
		}
		h := Attr(n, "href")
		u, _ := url.QueryUnescape(h)
		return strings.Contains(h, s) || strings.Contains(u, s)
	}
}

// First returns the first descendant (depth-first, including n) matching m.
func First(n *html.Node, m Match) *html.Node {
	if n == nil {
		return nil
	}
	if m(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := First(c, m); f != nil {
			return f
		}
	}
	return nil
}

// All returns every matching descendant without descending into matches.
func All(n *html.Node, m Match) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if m(n) {
			out = append(out, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	if n != nil {
		walk(n)
	}
	return out
}

// Children returns the direct children matching m.
func Children(n *html.Node, m Match) []*html.Node {
	var out []*html.Node
	if n == nil {
		return nil
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if m(c) {
			out = append(out, c)
		}
	}
	return out
}

// Cells returns the td/th cells of a table row.
func Cells(tr *html.Node) []*html.Node { return Children(tr, Tag("td", "th")) }

// Rows returns the data rows of a table: all <tr> in tbody (or the table when
// there is no tbody) that contain at least one <td>.
func Rows(table *html.Node) []*html.Node {
	var out []*html.Node
	for _, tr := range All(table, Tag("tr")) {
		if len(Children(tr, Tag("td"))) > 0 {
			out = append(out, tr)
		}
	}
	return out
}

func Attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func HasAttr(n *html.Node, key string) bool {
	if n == nil {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func HasClass(n *html.Node, c string) bool {
	for _, f := range strings.Fields(Attr(n, "class")) {
		if f == c {
			return true
		}
	}
	return false
}

// RawText concatenates all text nodes below n, skipping script/style.
func RawText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			sb.WriteString(n.Data)
		case html.ElementNode:
			if n.Data == "script" || n.Data == "style" {
				return
			}
			if n.Data == "br" {
				sb.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	if n != nil {
		walk(n)
	}
	return sb.String()
}

// Text returns the visible text of n with all whitespace collapsed to single spaces.
func Text(n *html.Node) string { return Collapse(RawText(n)) }

var blockTags = map[string]bool{"p": true, "div": true, "li": true, "tr": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "ul": true, "ol": true, "table": true, "section": true, "dd": true, "dt": true}

// LinesText keeps line structure (br and block elements become newlines) but
// collapses whitespace within each line and drops empty lines.
func LinesText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			// Source newlines are formatting, not content.
			sb.WriteString(strings.NewReplacer("\r", " ", "\n", " ").Replace(n.Data))
		case html.ElementNode:
			if n.Data == "script" || n.Data == "style" {
				return
			}
			if n.Data == "br" || blockTags[n.Data] {
				sb.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			sb.WriteString("\n")
		}
	}
	if n != nil {
		walk(n)
	}
	var lines []string
	for _, l := range strings.Split(sb.String(), "\n") {
		if l = Collapse(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

func Collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// AbsURL decodes HTML entities left in an href and resolves it against base.
func AbsURL(base, href string) string {
	href = strings.TrimSpace(strings.ReplaceAll(href, "&amp;", "&"))
	if href == "" || strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return strings.TrimRight(base, "/") + href
}

// QueryParam returns a query parameter, matching both the bare name and the
// TYPO3 tx_ext_plugin[name] notation.
func QueryParam(rawURL, param string) string {
	u, err := url.Parse(strings.ReplaceAll(rawURL, "&amp;", "&"))
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

// Action returns the TYPO3 Extbase action of a link ("" if none).
func Action(rawURL string) string { return QueryParam(rawURL, "action") }

// Main returns the <main> element (the CIS content area) or the document.
func Main(doc *html.Node) *html.Node {
	if m := First(doc, Tag("main")); m != nil {
		return m
	}
	return doc
}
