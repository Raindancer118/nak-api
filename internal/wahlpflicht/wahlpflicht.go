package wahlpflicht

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"golang.org/x/net/html"
)

const (
	listPath    = "/studium/bachelor/wahlpflichtkurse/wahlpflichtkurse-waehlen"
	extPrefix   = "tx_nawahlpflichtmodule_na_wahlpflichtmodule"
	curriculumID = "161"
)

type Module struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ModuleDetail struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Lernziele   string   `json:"lernziele"`
	Methoden    string   `json:"methoden"`
	Dozenten    []string `json:"dozenten"`
	Credits     string   `json:"credits"`
	Pruefung    string   `json:"pruefung"`
	Termine     string   `json:"termine"`
	SelectAvail bool     `json:"select_available"`
}

// FetchModules returns available Wahlpflicht modules by reading the list page.
func FetchModules(c *client.Client) ([]Module, error) {
	resp, err := c.Get(listPath)
	if err != nil {
		return nil, fmt.Errorf("fetch wahlpflicht list: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseModuleList(string(body)), nil
}

func parseModuleList(body string) []Module {
	doc, _ := html.Parse(strings.NewReader(body))
	var modules []Module
	seen := map[string]bool{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			action := attr(n, "action")
			if strings.Contains(action, extPrefix) {
				id := extractInputVal(n, extPrefix+"[id]")
				if id != "" && !seen[id] {
					seen[id] = true
					// Title is in a heading near the form — extract from parent context
					modules = append(modules, Module{ID: id})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return modules
}

// FetchDetail fetches the detail page for a module and returns structured info.
func FetchDetail(c *client.Client, moduleID string) (*ModuleDetail, error) {
	hiddenFields, postURL, err := getListHiddenFields(c)
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	for k, v := range hiddenFields {
		data.Set(k, v)
	}
	data.Set(extPrefix+"[id]", moduleID)
	data.Set(extPrefix+"[curriculumId]", curriculumID)

	resp, err := c.PostFormURL(postURL, data)
	if err != nil {
		return nil, fmt.Errorf("fetch wahlpflicht detail: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseModuleDetail(moduleID, string(body)), nil
}

// Select submits the Wahlpflicht module selection. Returns an error if the
// selection form is not yet available (period not open).
func Select(c *client.Client, moduleID string) (string, error) {
	// First get the detail page to find the selection form
	detail, err := FetchDetail(c, moduleID)
	if err != nil {
		return "", err
	}
	if !detail.SelectAvail {
		return "", fmt.Errorf("selection not yet available — wait until the selection period opens")
	}

	// Get fresh hidden fields from the detail page
	hiddenFields, postURL, err := getListHiddenFields(c)
	if err != nil {
		return "", err
	}

	// POST with the selection action
	data := url.Values{}
	for k, v := range hiddenFields {
		data.Set(k, v)
	}
	data.Set(extPrefix+"[id]", moduleID)
	data.Set(extPrefix+"[curriculumId]", curriculumID)
	// Action for the actual selection (likely "choose" or "save" — will be in the form)
	data.Set(extPrefix+"[action]", "choose")

	resp, err := c.PostFormURL(postURL, data)
	if err != nil {
		return "", fmt.Errorf("select module: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// Try to extract a success/confirmation message
	return extractFlashMessage(string(body)), nil
}

func getListHiddenFields(c *client.Client) (map[string]string, string, error) {
	resp, err := c.Get(listPath)
	if err != nil {
		return nil, "", fmt.Errorf("fetch list page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	doc, _ := html.Parse(strings.NewReader(string(body)))
	fields := map[string]string{}
	postURL := ""

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			action := attr(n, "action")
			if strings.Contains(action, extPrefix) {
				if !strings.HasPrefix(action, "http") {
					action = client.BaseURL + action
				}
				postURL = action
				var walkI func(*html.Node)
				walkI = func(n *html.Node) {
					if n.Type == html.ElementNode && n.Data == "input" && attr(n, "type") == "hidden" {
						name := attr(n, "name")
						if name != "" && strings.Contains(name, extPrefix) {
							fields[name] = attr(n, "value")
						}
					}
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						walkI(c)
					}
				}
				walkI(n)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if postURL == "" {
		return nil, "", fmt.Errorf("wahlpflicht form not found on list page")
	}
	return fields, postURL, nil
}

func parseModuleDetail(id, body string) *ModuleDetail {
	d := &ModuleDetail{ID: id}

	doc, _ := html.Parse(strings.NewReader(body))

	// Title from h2 (first non-nav h2)
	var walkH func(*html.Node)
	walkH = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "h2" && d.Title == "" {
			t := strings.TrimSpace(innerText(n))
			if t != "" && !strings.Contains(t, "NORDAKADEMIE") && !strings.Contains(t, "Social Media") {
				d.Title = t
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkH(c)
		}
	}
	walkH(doc)

	// Extract labeled sections from the page
	var prev string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := n.Data
			text := strings.TrimSpace(innerText(n))

			switch tag {
			case "dt", "strong", "b", "th":
				prev = strings.ToLower(text)
			case "dd", "td":
				if text == "" {
					break
				}
				applyField(d, prev, text)
				prev = ""
			case "p":
				if len(text) > 10 && prev != "" {
					applyField(d, prev, text)
					prev = ""
				}
			}

			// Check for selection form/button
			if tag == "form" {
				action := attr(n, "action")
				if strings.Contains(action, "choose") || strings.Contains(action, "save") || strings.Contains(action, "select") {
					d.SelectAvail = true
				}
			}
			if tag == "button" || tag == "input" {
				val := strings.ToLower(attr(n, "value") + " " + text)
				if strings.Contains(val, "wählen") || strings.Contains(val, "auswählen") || strings.Contains(val, "anmeld") {
					d.SelectAvail = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return d
}

func applyField(d *ModuleDetail, key, val string) {
	switch {
	case strings.Contains(key, "dozent"):
		if !containsStr(d.Dozenten, val) {
			d.Dozenten = append(d.Dozenten, val)
		}
	case strings.Contains(key, "credit") || key == "cp":
		if d.Credits == "" {
			d.Credits = val
		}
	case strings.Contains(key, "prüfung"):
		if d.Pruefung == "" {
			d.Pruefung = val
		}
	case strings.Contains(key, "termin"):
		if d.Termine == "" {
			d.Termine = val
		}
	case strings.Contains(key, "lernziel"):
		if d.Lernziele == "" {
			d.Lernziele = val
		}
	case strings.Contains(key, "lehr") || strings.Contains(key, "methode"):
		if d.Methoden == "" {
			d.Methoden = val
		}
	case strings.Contains(key, "beschreibung") || strings.Contains(key, "inhalt"):
		if d.Description == "" {
			d.Description = val
		}
	}
}

func extractFlashMessage(body string) string {
	doc, _ := html.Parse(strings.NewReader(body))
	var msg string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			cls := attr(n, "class")
			if strings.Contains(cls, "flash") || strings.Contains(cls, "alert") || strings.Contains(cls, "message") || strings.Contains(cls, "success") {
				text := strings.TrimSpace(innerText(n))
				if text != "" && msg == "" {
					msg = text
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if msg == "" {
		msg = "Request submitted — check CIS for confirmation."
	}
	return msg
}

func extractInputVal(n *html.Node, name string) string {
	var val string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" && attr(n, "name") == name {
			val = attr(n, "value")
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return val
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

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
