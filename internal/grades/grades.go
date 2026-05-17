package grades

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/scraper"
)

type Grade struct {
	Module   string
	Grade    string
	Credits  string
	Semester string
	Status   string
}

// FetchOverview fetches the Leistungsübersicht page and returns all transcript
// links found there (including their cHash — required by TYPO3).
func FetchOverview(c *client.Client) ([]string, error) {
	resp, err := c.Get("/mein-profil/mein-postfach/leistungsuebersicht")
	if err != nil {
		return nil, fmt.Errorf("fetch overview: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	links, err := scraper.ExtractLinks(strings.NewReader(string(body)), "tx_nagrades_nagradesmodules")
	if err != nil {
		return nil, fmt.Errorf("parse overview links: %w", err)
	}
	// Make relative URLs absolute
	for i, l := range links {
		if !strings.HasPrefix(l, "http") {
			links[i] = client.BaseURL + l
		}
	}
	return links, nil
}

// FetchTranscript fetches grades from a transcript URL (obtained via FetchOverview).
// It returns parsed grade rows from all tables on the page.
func FetchTranscript(c *client.Client, transcriptURL string) ([]Grade, error) {
	resp, err := c.GetURL(transcriptURL)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	tables, err := scraper.ExtractTables(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse transcript tables: %w", err)
	}

	var grades []Grade
	for _, t := range tables {
		idx := detectGradeTable(t.Headers)
		if idx == nil {
			continue
		}
		for _, row := range t.Rows {
			if len(row) < 2 {
				continue
			}
			g := Grade{}
			if idx.module >= 0 && idx.module < len(row) {
				g.Module = row[idx.module]
			}
			if idx.grade >= 0 && idx.grade < len(row) {
				g.Grade = row[idx.grade]
			}
			if idx.credits >= 0 && idx.credits < len(row) {
				g.Credits = row[idx.credits]
			}
			if idx.semester >= 0 && idx.semester < len(row) {
				g.Semester = row[idx.semester]
			}
			if idx.status >= 0 && idx.status < len(row) {
				g.Status = row[idx.status]
			}
			grades = append(grades, g)
		}
	}
	return grades, nil
}

// FetchAll is a convenience that runs FetchOverview + FetchTranscript for
// every transcript link found. It uses the first link with the given lang
// param (defaults to "de").
func FetchAll(c *client.Client, lang string) ([]Grade, []string, error) {
	if lang == "" {
		lang = "de"
	}
	links, err := FetchOverview(c)
	if err != nil {
		return nil, nil, err
	}
	if len(links) == 0 {
		return nil, links, fmt.Errorf("no transcript links found — are you logged in?")
	}

	// Prefer link matching the requested language
	chosen := links[0]
	for _, l := range links {
		if strings.Contains(l, "lang%5D="+lang) || strings.Contains(l, "lang]="+lang) {
			chosen = l
			break
		}
	}

	// Ensure lang parameter is set
	u, err := url.Parse(chosen)
	if err != nil {
		return nil, links, err
	}
	q := u.Query()
	q.Set("tx_nagrades_nagradesmodules[lang]", lang)
	u.RawQuery = q.Encode()

	gs, err := FetchTranscript(c, u.String())
	return gs, links, err
}

type colIdx struct {
	module, grade, credits, semester, status int
}

var gradeHeaderKeywords = map[string][]string{
	"module":   {"modul", "veranstaltung", "fach", "subject", "lehrveranstaltung"},
	"grade":    {"note", "grade", "ergebnis", "bewertung"},
	"credits":  {"cp", "ects", "credits", "punkte", "leistungspunkte"},
	"semester": {"semester", "sem."},
	"status":   {"status", "angerechnet", "prüfungsstatus"},
}

func detectGradeTable(headers []string) *colIdx {
	if len(headers) == 0 {
		return nil
	}
	idx := &colIdx{
		module: -1, grade: -1, credits: -1, semester: -1, status: -1,
	}
	for i, h := range headers {
		lower := strings.ToLower(h)
		for _, kw := range gradeHeaderKeywords["module"] {
			if strings.Contains(lower, kw) {
				idx.module = i
			}
		}
		for _, kw := range gradeHeaderKeywords["grade"] {
			if strings.Contains(lower, kw) {
				idx.grade = i
			}
		}
		for _, kw := range gradeHeaderKeywords["credits"] {
			if strings.Contains(lower, kw) {
				idx.credits = i
			}
		}
		for _, kw := range gradeHeaderKeywords["semester"] {
			if strings.Contains(lower, kw) {
				idx.semester = i
			}
		}
		for _, kw := range gradeHeaderKeywords["status"] {
			if strings.Contains(lower, kw) {
				idx.status = i
			}
		}
	}
	// Must have at least a module or grade column to count as a grades table
	if idx.module == -1 && idx.grade == -1 {
		return nil
	}
	return idx
}
