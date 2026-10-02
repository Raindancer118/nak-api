package moodle

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

var blockTags = map[string]bool{"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "tr": true, "table": true, "ul": true, "ol": true, "blockquote": true, "pre": true, "section": true,
	"article": true, "header": true, "footer": true, "dt": true, "dd": true}

var skipTags = map[string]bool{"script": true, "style": true, "img": true, "svg": true, "noscript": true, "head": true}

// Inline renders HTML as one line ("" for blank input).
func Inline(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(htmlx.MustParse(s))
	return htmlx.Collapse(strings.ReplaceAll(sb.String(), "  ", " "))
}

// InlineMax is Inline truncated to max characters.
func InlineMax(s string, max int) string { return Truncate(Inline(s), max) }

var (
	spaceBeforeNL = regexp.MustCompile(`[ \t]+\n`)
	spaceAfterNL  = regexp.MustCompile(`\n[ \t]+`)
	manyNL        = regexp.MustCompile(`\n{3,}`)
	ws            = regexp.MustCompile(`\s+`)
)

// Blocks keeps paragraph and list structure as line breaks.
func Blocks(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	doc := htmlx.MustParse(s)
	body := htmlx.First(doc, htmlx.Tag("body"))
	if body == nil {
		body = doc
	}
	var sb strings.Builder
	last := func() byte {
		if sb.Len() == 0 {
			return '\n'
		}
		return sb.String()[sb.Len()-1]
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			t := ws.ReplaceAllString(n.Data, " ")
			if strings.TrimSpace(t) != "" || (last() != ' ' && last() != '\n') {
				sb.WriteString(t)
			}
		case html.ElementNode:
			if skipTags[n.Data] {
				return
			}
			switch {
			case n.Data == "br":
				sb.WriteByte('\n')
			case n.Data == "li":
				sb.WriteString("\n- ")
			case blockTags[n.Data]:
				sb.WriteByte('\n')
			case n.Data == "td" || n.Data == "th":
				sb.WriteString(" | ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			sb.WriteByte('\n')
		}
	}
	walk(body)
	t := sb.String()
	t = spaceBeforeNL.ReplaceAllString(t, "\n")
	t = spaceAfterNL.ReplaceAllString(t, "\n")
	t = manyNL.ReplaceAllString(t, "\n\n")
	return strings.TrimSpace(strings.ReplaceAll(t, " ", " "))
}

// Truncate shortens s to max characters with a marker.
func Truncate(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	return string(r[:max]) + fmt.Sprintf(" …[gekürzt, %d Zeichen]", len(r))
}
