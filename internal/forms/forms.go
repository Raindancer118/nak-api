// Package forms rebuilds HTML form submissions the way a browser would, so
// TYPO3/Extbase accepts them (__trustedProperties, hidden checkbox twins, the
// clicked submit button). A Submission is built and previewed first; sending it
// is always a separate, explicit client.Write* call.
package forms

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

type Option struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Selected bool   `json:"selected,omitempty"`
}

type Field struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"` // hidden, text, email, checkbox, radio, select, textarea, file, submit, …
	Value    string   `json:"value"`
	Label    string   `json:"label,omitempty"`
	Checked  bool     `json:"checked,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
	Options  []Option `json:"options,omitempty"`
}

type Form struct {
	Action    string     `json:"action"`
	Method    string     `json:"method"`
	Multipart bool       `json:"multipart"`
	Fields    []*Field   `json:"fields"`
	Node      *html.Node `json:"-"`
}

type Change struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// Submission is a fully built request that has not been sent.
type Submission struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Multipart bool              `json:"multipart"`
	Fields    []client.KV       `json:"fields"`
	Files     []client.FilePart `json:"-"`
	Changes   []Change          `json:"changes"`
	Button    string            `json:"button"`
}

// Parse extracts every form below n; relative actions are resolved against base.
func Parse(n *html.Node, base string) []*Form {
	labels := map[string]string{}
	for _, l := range htmlx.All(n, htmlx.Tag("label")) {
		if id := htmlx.Attr(l, "for"); id != "" {
			labels[id] = strings.TrimSuffix(htmlx.Text(l), ":")
		}
	}
	var out []*Form
	for _, fn := range htmlx.All(n, htmlx.Tag("form")) {
		f := &Form{
			Action:    htmlx.AbsURL(base, htmlx.Attr(fn, "action")),
			Method:    strings.ToUpper(htmlx.Attr(fn, "method")),
			Multipart: strings.Contains(strings.ToLower(htmlx.Attr(fn, "enctype")), "multipart"),
			Node:      fn,
		}
		if f.Method == "" {
			f.Method = "GET"
		}
		var walk func(*html.Node)
		walk = func(c *html.Node) {
			if c.Type == html.ElementNode {
				switch c.Data {
				case "input", "select", "textarea", "button":
					if fld := parseField(c); fld != nil {
						fld.Label = labels[htmlx.Attr(c, "id")]
						f.Fields = append(f.Fields, fld)
						if fld.Kind == "file" {
							f.Multipart = true
						}
					}
					if c.Data != "button" {
						return
					}
				}
			}
			for ch := c.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
		}
		walk(fn)
		out = append(out, f)
	}
	return out
}

func parseField(n *html.Node) *Field {
	f := &Field{Name: htmlx.Attr(n, "name"), Disabled: htmlx.HasAttr(n, "disabled")}
	switch n.Data {
	case "input":
		f.Kind = strings.ToLower(htmlx.Attr(n, "type"))
		if f.Kind == "" {
			f.Kind = "text"
		}
		f.Value = htmlx.Attr(n, "value")
		if f.Kind == "checkbox" || f.Kind == "radio" {
			f.Checked = htmlx.HasAttr(n, "checked")
			if !htmlx.HasAttr(n, "value") {
				f.Value = "on"
			}
		}
		if f.Kind == "image" || f.Kind == "button" || f.Kind == "reset" {
			return nil
		}
	case "button":
		t := strings.ToLower(htmlx.Attr(n, "type"))
		if t != "" && t != "submit" {
			return nil
		}
		f.Kind = "submit"
		f.Value = htmlx.Attr(n, "value")
		if f.Value == "" {
			f.Value = htmlx.Text(n)
		}
	case "textarea":
		f.Kind = "textarea"
		f.Value = htmlx.RawText(n)
	case "select":
		f.Kind = "select"
		for _, o := range htmlx.All(n, htmlx.Tag("option")) {
			v := htmlx.Attr(o, "value")
			if !htmlx.HasAttr(o, "value") {
				v = htmlx.Text(o)
			}
			opt := Option{Value: v, Label: htmlx.Text(o), Selected: htmlx.HasAttr(o, "selected")}
			f.Options = append(f.Options, opt)
			if opt.Selected {
				f.Value = v
			}
		}
		if f.Value == "" && len(f.Options) > 0 && !anySelected(f.Options) {
			f.Value = f.Options[0].Value
		}
	}
	return f
}

func anySelected(os []Option) bool {
	for _, o := range os {
		if o.Selected {
			return true
		}
	}
	return false
}

// FindByAction returns the first form whose (unescaped) action contains s.
func FindByAction(fs []*Form, s string) *Form {
	for _, f := range fs {
		u, _ := url.QueryUnescape(f.Action)
		if strings.Contains(f.Action, s) || strings.Contains(u, s) {
			return f
		}
	}
	return nil
}

// FindByField returns the first form that has a field matching key.
func FindByField(fs []*Form, key string) *Form {
	for _, f := range fs {
		if len(f.match(key)) > 0 {
			return f
		}
	}
	return nil
}

// match resolves a user key to the distinct field names it means: the exact
// name, a bracket suffix ("[email][privat]") or a dotted short form
// ("email.privat").
func (f *Form) match(key string) []string {
	suffix := key
	if !strings.Contains(key, "[") {
		suffix = "[" + strings.ReplaceAll(key, ".", "][") + "]"
	}
	seen := map[string]bool{}
	var names []string
	for _, fl := range f.Fields {
		if fl.Name == "" || seen[fl.Name] {
			continue
		}
		base := strings.TrimSuffix(fl.Name, "[]")
		if fl.Name == key || base == key || strings.HasSuffix(fl.Name, suffix) || strings.HasSuffix(base, suffix) {
			seen[fl.Name] = true
			names = append(names, fl.Name)
		}
	}
	return names
}

func (f *Form) resolve(key string) (string, error) {
	names := f.match(key)
	switch len(names) {
	case 0:
		return "", fmt.Errorf("form has no field %q", key)
	case 1:
		return names[0], nil
	default:
		return "", fmt.Errorf("field %q is ambiguous: %s", key, strings.Join(names, ", "))
	}
}

// Field returns the first visible (non-hidden) field for key, else any field.
func (f *Form) Field(key string) *Field {
	name, err := f.resolve(key)
	if err != nil {
		return nil
	}
	var fallback *Field
	for _, fl := range f.Fields {
		if fl.Name == name {
			if fl.Kind != "hidden" {
				return fl
			}
			if fallback == nil {
				fallback = fl
			}
		}
	}
	return fallback
}

// Submit builds the request a browser would send after the given edits and a
// click on button ("" = first named submit button). Every override is
// validated: unknown or disabled fields and invalid select options are errors.
func (f *Form) Submit(button string, set map[string]string, files ...client.FilePart) (*Submission, error) {
	resolved := map[string]string{}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name, err := f.resolve(k)
		if err != nil {
			return nil, err
		}
		resolved[name] = set[k]
	}

	btn, err := f.button(button)
	if err != nil {
		return nil, err
	}

	s := &Submission{Method: f.Method, URL: f.Action, Multipart: f.Multipart}
	if btn != nil {
		s.Button = btn.Value
	}
	applied := map[string]bool{}
	for _, fl := range f.Fields {
		if fl.Name == "" {
			continue
		}
		nv, overridden := resolved[fl.Name]
		if overridden && fl.Disabled {
			return nil, fmt.Errorf("field %s is disabled in the form", fl.Name)
		}
		if fl.Disabled {
			continue
		}
		switch fl.Kind {
		case "submit":
			if fl == btn {
				s.Fields = append(s.Fields, client.KV{Name: fl.Name, Value: fl.Value})
			}
		case "file":
			// filled from files below
		case "checkbox", "radio":
			on := fl.Checked
			if overridden {
				on = nv != "" && nv != "0" && strings.ToLower(nv) != "false" && (fl.Kind == "checkbox" || nv == fl.Value)
				if fl.Kind == "checkbox" && on && nv != fl.Value && nv != "1" && strings.ToLower(nv) != "true" && nv != "on" {
					return nil, fmt.Errorf("checkbox %s only accepts %q (or empty to uncheck)", fl.Name, fl.Value)
				}
				if !applied[fl.Name] {
					applied[fl.Name] = true
					s.Changes = appendChange(s.Changes, fl.Name, boolStr(fl.Checked, fl.Value), boolStr(on, fl.Value))
				}
			}
			if on {
				s.Fields = append(s.Fields, client.KV{Name: fl.Name, Value: fl.Value})
			}
		case "select":
			v := fl.Value
			if overridden {
				if !hasOption(fl.Options, nv) {
					return nil, fmt.Errorf("invalid value %q for %s; allowed: %s", nv, fl.Name, optionList(fl.Options))
				}
				s.Changes = appendChange(s.Changes, fl.Name, fl.Value, nv)
				applied[fl.Name] = true
				v = nv
			}
			s.Fields = append(s.Fields, client.KV{Name: fl.Name, Value: v})
		case "hidden":
			v := fl.Value
			if overridden && !hasCheckboxTwin(f, fl.Name) {
				s.Changes = appendChange(s.Changes, fl.Name, fl.Value, nv)
				applied[fl.Name] = true
				v = nv
			}
			s.Fields = append(s.Fields, client.KV{Name: fl.Name, Value: v})
		default:
			v := fl.Value
			if overridden && !applied[fl.Name] {
				s.Changes = appendChange(s.Changes, fl.Name, fl.Value, nv)
				applied[fl.Name] = true
				v = nv
			}
			s.Fields = append(s.Fields, client.KV{Name: fl.Name, Value: v})
		}
	}

	for _, fp := range files {
		name, err := f.resolve(fp.Field)
		if err != nil {
			return nil, err
		}
		if fl := f.fieldByName(name); fl == nil || fl.Kind != "file" {
			return nil, fmt.Errorf("%s is not a file field", name)
		}
		fp.Field = name
		s.Files = append(s.Files, fp)
		s.Changes = appendChange(s.Changes, name, "", fmt.Sprintf("%s (%d bytes)", fp.Filename, len(fp.Data)))
		s.Multipart = true
	}
	return s, nil
}

func (f *Form) fieldByName(name string) *Field {
	for _, fl := range f.Fields {
		if fl.Name == name {
			return fl
		}
	}
	return nil
}

func (f *Form) button(key string) (*Field, error) {
	var named []*Field
	for _, fl := range f.Fields {
		if fl.Kind == "submit" && !fl.Disabled {
			named = append(named, fl)
		}
	}
	if key == "" {
		for _, b := range named {
			if b.Name != "" {
				return b, nil
			}
		}
		return nil, nil // nameless buttons are not sent
	}
	suffix := "[" + strings.ReplaceAll(key, ".", "][") + "]"
	for _, b := range named {
		if b.Name == key || strings.HasSuffix(b.Name, suffix) || strings.EqualFold(b.Value, key) {
			return b, nil
		}
	}
	return nil, fmt.Errorf("form has no submit button %q", key)
}

func hasCheckboxTwin(f *Form, name string) bool {
	for _, fl := range f.Fields {
		if fl.Name == name && fl.Kind == "checkbox" {
			return true
		}
	}
	return false
}

func hasOption(os []Option, v string) bool {
	for _, o := range os {
		if o.Value == v {
			return true
		}
	}
	return false
}

func optionList(os []Option) string {
	var parts []string
	for _, o := range os {
		parts = append(parts, fmt.Sprintf("%q (%s)", o.Value, o.Label))
	}
	return strings.Join(parts, ", ")
}

func boolStr(on bool, v string) string {
	if on {
		return v
	}
	return ""
}

func appendChange(cs []Change, name, old, nw string) []Change {
	if old == nw {
		return cs
	}
	return append(cs, Change{Field: name, Old: old, New: nw})
}

// Values returns the urlencoded body (only for non-multipart submissions).
func (s *Submission) Values() url.Values {
	v := url.Values{}
	for _, f := range s.Fields {
		v.Add(f.Name, f.Value)
	}
	return v
}

// QueryURL returns the URL a browser requests for a GET form: per the HTML
// spec the fields REPLACE the action's query string.
func (s *Submission) QueryURL() string {
	u, err := url.Parse(s.URL)
	if err != nil {
		return s.URL
	}
	q := url.Values{}
	for _, f := range s.Fields {
		q.Add(f.Name, f.Value)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Send performs the submission through the client's write path.
func (s *Submission) Send(c *client.Client) (*client.Page, error) {
	if s.Multipart {
		return c.WriteMultipart(s.URL, s.Fields, s.Files)
	}
	if s.Method == "GET" {
		return c.WriteGet(s.QueryURL())
	}
	return c.WritePost(s.URL, s.Values())
}

// Preview is a human-readable description: target, changed fields and the
// visible values that will be sent. TYPO3 integrity tokens are summarised.
func (s *Submission) Preview() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", s.Method, stripCHash(s.URL))
	if s.Button != "" {
		fmt.Fprintf(&b, "Button: %q\n", s.Button)
	}
	if len(s.Changes) == 0 {
		b.WriteString("Änderungen: keine (Formular unverändert)\n")
	} else {
		b.WriteString("Änderungen:\n")
		for _, c := range s.Changes {
			fmt.Fprintf(&b, "  %s: %q → %q\n", shortName(c.Field), c.Old, c.New)
		}
	}
	b.WriteString("Gesendete Felder:\n")
	tokens := 0
	for _, f := range s.Fields {
		if strings.Contains(f.Name, "__") {
			tokens++
			continue
		}
		fmt.Fprintf(&b, "  %s = %q\n", shortName(f.Name), f.Value)
	}
	for _, f := range s.Files {
		fmt.Fprintf(&b, "  %s = <Datei %s, %d Bytes>\n", shortName(f.Field), f.Filename, len(f.Data))
	}
	if tokens > 0 {
		fmt.Fprintf(&b, "  (+%d TYPO3-Integritätsfelder unverändert aus dem Formular)\n", tokens)
	}
	return b.String()
}

func stripCHash(u string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	q := pu.Query()
	q.Del("cHash")
	pu.RawQuery = q.Encode()
	s, _ := url.QueryUnescape(pu.String())
	return s
}

// shortName drops the tx_extension prefix: tx_x[email][privat] -> email.privat
func shortName(n string) string {
	i := strings.Index(n, "[")
	if i < 0 || !strings.HasPrefix(n, "tx_") {
		return n
	}
	return strings.NewReplacer("][", ".", "[", "", "]", "").Replace(n[i:])
}

// Editable lists the fields a user could change (no hidden/submit/disabled).
func (f *Form) Editable() []*Field {
	var out []*Field
	seen := map[string]bool{}
	for _, fl := range f.Fields {
		if fl.Name == "" || fl.Disabled || fl.Kind == "hidden" || fl.Kind == "submit" || seen[fl.Name] {
			continue
		}
		seen[fl.Name] = true
		out = append(out, fl)
	}
	return out
}

// ShortName is exported for callers rendering field lists.
func ShortName(n string) string { return shortName(n) }
