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
	ModuleNumber string `json:"module_number,omitempty"`
	Module       string `json:"module"`
	ExamDate     string `json:"exam_date,omitempty"`
	Grade        string `json:"grade"`
	Credits      string `json:"credits"`
	Semester     string `json:"semester,omitempty"`
	Status       string `json:"status,omitempty"`
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

// FetchDirect parses grades directly from the Leistungsübersicht page
// without following transcript links. This is the primary fetch method since
// the grade table is embedded on the overview page itself.
func FetchDirect(c *client.Client) ([]Grade, error) {
	resp, err := c.Get("/mein-profil/mein-postfach/leistungsuebersicht")
	if err != nil {
		return nil, fmt.Errorf("fetch leistungsuebersicht: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	tables, err := scraper.ExtractTables(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse tables: %w", err)
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
			if idx.moduleNumber >= 0 && idx.moduleNumber < len(row) {
				g.ModuleNumber = row[idx.moduleNumber]
			}
			if idx.module >= 0 && idx.module < len(row) {
				g.Module = row[idx.module]
			}
			if idx.examDate >= 0 && idx.examDate < len(row) {
				g.ExamDate = row[idx.examDate]
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
			if idx.moduleNumber >= 0 && idx.moduleNumber < len(row) {
				g.ModuleNumber = row[idx.moduleNumber]
			}
			if idx.module >= 0 && idx.module < len(row) {
				g.Module = row[idx.module]
			}
			if idx.examDate >= 0 && idx.examDate < len(row) {
				g.ExamDate = row[idx.examDate]
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

// FetchAll fetches grades by first trying the direct page approach (table
// embedded on Leistungsübersicht), then falling back to transcript links.
func FetchAll(c *client.Client, lang string) ([]Grade, []string, error) {
	if lang == "" {
		lang = "de"
	}

	// Primary: grades are embedded directly on the overview page
	gs, err := FetchDirect(c)
	if err != nil {
		return nil, nil, err
	}
	if len(gs) > 0 {
		return gs, nil, nil
	}

	// Fallback: follow transcript links (print/PDF view)
	links, err := FetchOverview(c)
	if err != nil {
		return nil, nil, err
	}
	if len(links) == 0 {
		return nil, nil, fmt.Errorf("keine Noten gefunden — bitte einloggen oder Leistungsübersicht prüfen")
	}

	chosen := links[0]
	for _, l := range links {
		if strings.Contains(l, "lang%5D="+lang) || strings.Contains(l, "lang]="+lang) {
			chosen = l
			break
		}
	}

	u, err := url.Parse(chosen)
	if err != nil {
		return nil, links, err
	}
	q := u.Query()
	q.Set("tx_nagrades_nagradesmodules[lang]", lang)
	u.RawQuery = q.Encode()

	gs, err = FetchTranscript(c, u.String())
	if err != nil {
		return nil, links, err
	}
	if len(gs) == 0 {
		return nil, links, fmt.Errorf("keine Noten gefunden — Seitenstruktur möglicherweise geändert")
	}
	return gs, links, nil
}

type colIdx struct {
	moduleNumber, module, examDate, grade, credits, semester, status int
}

var gradeHeaderKeywords = map[string][]string{
	"module_number": {"modulnummer"},
	"module":        {"bezeichnung", "modul", "veranstaltung", "fach", "subject", "lehrveranstaltung"},
	"exam_date":     {"prüfungsdatum", "pruefungsdatum", "datum"},
	"grade":         {"note", "grade", "ergebnis", "bewertung"},
	"credits":       {"credits", "cp", "ects", "punkte", "leistungspunkte"},
	"semester":      {"semester", "sem."},
	"status":        {"status", "angerechnet", "prüfungsstatus"},
}

func detectGradeTable(headers []string) *colIdx {
	if len(headers) == 0 {
		return nil
	}
	idx := &colIdx{
		moduleNumber: -1, module: -1, examDate: -1,
		grade: -1, credits: -1, semester: -1, status: -1,
	}
	for i, h := range headers {
		lower := strings.ToLower(h)
		for _, kw := range gradeHeaderKeywords["module_number"] {
			if strings.Contains(lower, kw) {
				idx.moduleNumber = i
			}
		}
		for _, kw := range gradeHeaderKeywords["module"] {
			if strings.Contains(lower, kw) {
				idx.module = i
			}
		}
		for _, kw := range gradeHeaderKeywords["exam_date"] {
			if strings.Contains(lower, kw) {
				idx.examDate = i
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
	// Must have at least a module/bezeichnung or grade/note column to count as a grades table
	if idx.module == -1 && idx.grade == -1 && idx.moduleNumber == -1 {
		return nil
	}
	return idx
}
