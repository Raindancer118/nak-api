// Package certs lists and downloads the online Studienbescheinigungen
// (tx_naconfirmationofenrollment). Each semester row links a German and an
// English PDF that the CIS renders on request (action=document).
package certs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/htmlx"
)

const PagePath = "/mein-profil/mein-postfach/online-bescheinigungen"

type Certificate struct {
	Name        string `json:"name"`             // e.g. "Studienbescheinigung WS 2025 (de)"
	Semester    string `json:"semester"`         // e.g. "WS 2025"
	Period      string `json:"period,omitempty"` // e.g. "01.10.2025 - 31.03.2026"
	Lang        string `json:"lang"`
	DownloadURL string `json:"download_url"`
}

var semRe = regexp.MustCompile(`^(.+?)\s*\((.+)\)$`)

func FetchList(c *client.Client) ([]Certificate, error) {
	p, err := c.Page(PagePath)
	if err != nil {
		return nil, fmt.Errorf("online-bescheinigungen: %w", err)
	}
	return Parse(p.Body, c.Base), nil
}

func Parse(body, base string) []Certificate {
	doc := htmlx.Main(htmlx.MustParse(body))
	var out []Certificate
	seen := map[string]bool{}
	for _, tr := range htmlx.All(doc, htmlx.Tag("tr")) {
		cells := htmlx.Children(tr, htmlx.Tag("td"))
		if len(cells) == 0 {
			continue
		}
		label := htmlx.Text(cells[0])
		sem, period := label, ""
		if m := semRe.FindStringSubmatch(label); m != nil {
			sem, period = m[1], m[2]
		}
		for _, a := range htmlx.All(tr, htmlx.HrefContains("[action]=document")) {
			u := htmlx.AbsURL(base, htmlx.Attr(a, "href"))
			lang := htmlx.QueryParam(u, "lang")
			if seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, Certificate{Name: fmt.Sprintf("Studienbescheinigung %s (%s)", sem, lang), Semester: sem, Period: period, Lang: lang, DownloadURL: u})
		}
	}
	// Other documents offered on the page (static PDFs).
	for _, a := range htmlx.All(doc, htmlx.HrefContains("eID=dumpFile")) {
		u := htmlx.AbsURL(base, htmlx.Attr(a, "href"))
		if !seen[u] {
			seen[u] = true
			out = append(out, Certificate{Name: htmlx.Text(a), DownloadURL: u})
		}
	}
	return out
}

// Find picks a certificate by semester (case-insensitive prefix, "" = newest) and language.
func Find(list []Certificate, semester, lang string) (*Certificate, error) {
	if lang == "" {
		lang = "de"
	}
	var match *Certificate
	for i := range list {
		c := &list[i]
		if c.Lang != lang {
			continue
		}
		if semester == "" || strings.EqualFold(c.Semester, semester) || strings.HasPrefix(strings.ToLower(c.Semester), strings.ToLower(semester)) {
			match = c // rows are chronological: the last match is the newest
		}
	}
	if match == nil {
		return nil, fmt.Errorf("no certificate for semester %q in %q", semester, lang)
	}
	return match, nil
}

// Download fetches the PDF. Rendering it on the server changes nothing.
func Download(c *client.Client, downloadURL string) ([]byte, string, error) {
	data, ct, _, err := c.Download(downloadURL)
	if err != nil {
		return nil, ct, err
	}
	if !strings.Contains(ct, "pdf") && strings.Contains(string(data[:min(len(data), 512)]), "<html") {
		return nil, ct, fmt.Errorf("expected a PDF, got an HTML page (certificate not yet available?)")
	}
	return data, ct, nil
}
