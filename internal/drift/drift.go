// Package drift marks failures caused by the CIS changing its pages and
// describes a page's structure without any of its content, so the
// description can go into a public GitHub issue.
package drift

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

var ErrDrift = errors.New("the CIS page no longer has the expected structure")

type Error struct {
	Page        string
	Expected    string
	Fingerprint *Print
}

func (e *Error) Error() string {
	return fmt.Sprintf("CIS structure changed on %s: expected %s", e.Page, e.Expected)
}

func (e *Error) Unwrap() error { return ErrDrift }

// New records that page (a path) lacked what the parser expected.
func New(page, expected, body string) error {
	fp := Fingerprint(body)
	return &Error{Page: page, Expected: expected, Fingerprint: &fp}
}

// Print is the content-free outline of a page.
type Print struct {
	Title       string   `json:"title"`
	TableHeads  []string `json:"table_headers"`
	DataLabels  []string `json:"data_labels"`
	FormActions []string `json:"form_actions"`
	FieldNames  []string `json:"form_fields"`
	Classes     []string `json:"main_classes"`
	IDs         []string `json:"ids"`
	Plugins     []string `json:"typo3_plugins"`
	Bytes       int      `json:"bytes"`
}

// Fingerprint keeps labels a developer needs (table headers, data-label
// names, field names, classes, TYPO3 plugin keys) and drops every cell,
// heading and attribute value that could carry personal data.
func Fingerprint(body string) Print {
	doc := htmlx.MustParse(body)
	main := htmlx.Main(doc)
	p := Print{Bytes: len(body)}
	if t := htmlx.First(doc, htmlx.Tag("title")); t != nil {
		p.Title = htmlx.Text(t)
	}
	set := func() map[string]bool { return map[string]bool{} }
	heads, labels, actions, fields, classes, ids, plugins := set(), set(), set(), set(), set(), set(), set()
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "th":
				if t := htmlx.Text(n); t != "" && len(t) < 40 {
					heads[t] = true
				}
			case "form":
				if u, err := url.Parse(strings.ReplaceAll(htmlx.Attr(n, "action"), "&amp;", "&")); err == nil {
					q := u.Query()
					act := ""
					for k, v := range q {
						if strings.HasSuffix(k, "[action]") {
							act = k[:strings.Index(k, "[")] + "[action]=" + v[0]
						}
					}
					actions[u.Path+" "+act] = true
				}
			case "input", "select", "textarea", "button":
				if nm := htmlx.Attr(n, "name"); nm != "" {
					fields[nm] = true
				}
			case "a":
				if u, err := url.Parse(strings.ReplaceAll(htmlx.Attr(n, "href"), "&amp;", "&")); err == nil {
					for k := range u.Query() {
						if strings.HasPrefix(k, "tx_") {
							plugins[k[:strings.IndexAny(k+"[", "[")]] = true
						}
					}
				}
			}
			if l := htmlx.Attr(n, "data-label"); l != "" {
				labels[l] = true
			}
			if id := htmlx.Attr(n, "id"); id != "" && len(id) < 40 {
				ids[id] = true
			}
			for _, c := range strings.Fields(htmlx.Attr(n, "class")) {
				classes[c] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(main)
	p.TableHeads, p.DataLabels, p.FormActions = keys(heads, 40), keys(labels, 40), keys(actions, 20)
	p.FieldNames, p.Classes, p.IDs, p.Plugins = keys(fields, 60), keys(classes, 60), keys(ids, 30), keys(plugins, 20)
	return p
}

func keys(m map[string]bool, max int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > max {
		out = append(out[:max], fmt.Sprintf("… (+%d)", len(m)-max))
	}
	return out
}

func (p Print) String() string {
	var b strings.Builder
	row := func(name string, v []string) {
		if len(v) > 0 {
			fmt.Fprintf(&b, "- **%s:** `%s`\n", name, strings.Join(v, "`, `"))
		}
	}
	fmt.Fprintf(&b, "- **title:** %s (%d bytes)\n", p.Title, p.Bytes)
	row("table headers", p.TableHeads)
	row("data-labels", p.DataLabels)
	row("form actions", p.FormActions)
	row("form fields", p.FieldNames)
	row("TYPO3 plugins", p.Plugins)
	row("ids", p.IDs)
	row("classes", p.Classes)
	return b.String()
}
