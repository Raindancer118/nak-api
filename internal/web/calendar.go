package web

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Raindancer118/nak-api/internal/tools"
)

// The calendar feed has its own secret: calendar apps cannot send cookies or
// headers, so the token is part of the URL — and can be rotated on its own
// without logging anybody out.
const calendarFile = "calendar-token"

func (s *Server) calendarToken() (string, error) {
	tok, _, err := loadOrCreateSecret(s.app.ConfigDir, calendarFile)
	return tok, err
}

func (s *Server) calendarInfo(w http.ResponseWriter, r *http.Request) {
	tok, err := s.calendarToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": "/calendar/" + tok + ".ics"})
}

func (s *Server) calendarRotate(w http.ResponseWriter, r *http.Request) {
	os.Remove(filepath.Join(s.app.ConfigDir, calendarFile))
	s.calendarInfo(w, r)
}

func (s *Server) calendarFeed(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	want, err := s.calendarToken()
	if err != nil || !strings.HasSuffix(name, ".ics") || !eq(strings.TrimSuffix(name, ".ics"), want) {
		http.NotFound(w, r)
		return
	}
	var cal icsWriter
	cal.line("BEGIN:VCALENDAR")
	cal.line("VERSION:2.0")
	cal.line("PRODID:-//naknak//NORDAKADEMIE CIS + Moodle//DE")
	cal.line("CALSCALE:GREGORIAN")
	cal.line("METHOD:PUBLISH")
	cal.prop("X-WR-CALNAME", "naknak")
	cal.prop("X-WR-TIMEZONE", s.app.Zone.String())
	cal.line("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	cal.line("X-PUBLISHED-TTL:PT1H")

	stamp := s.cfg.Now().UTC().Format("20060102T150405Z")
	if raw, err := s.cachedTool("nak_agenda", tools.Args{"days": 56}); err == nil {
		var ag struct {
			Days []struct {
				Events []agendaEvent `json:"events"`
			} `json:"days"`
		}
		json.Unmarshal(raw, &ag)
		for _, d := range ag.Days {
			for _, e := range d.Events {
				s.writeEvent(&cal, e, stamp)
			}
		}
	}
	if raw, err := s.cachedTool("nak_deadlines", tools.Args{"days": 60}); err == nil {
		var dl struct {
			Deadlines []struct {
				When, What, Kind, Source, Hint string
				ModuleNr                       string `json:"module_nr"`
			} `json:"deadlines"`
		}
		json.Unmarshal(raw, &dl)
		for _, d := range dl.Deadlines {
			// lectures, exams and Moodle dues already come from the agenda
			switch d.Kind {
			case "exam_registration", "exam_deregistration", "transfer", "graduation":
			default:
				continue
			}
			date, clock, _ := strings.Cut(strings.TrimSpace(d.When[strings.Index(d.When, " ")+1:]), " ")
			s.writeEvent(&cal, agendaEvent{Date: date, Start: clock, End: clock, Kind: "Frist", Title: d.What, ModuleNr: d.ModuleNr, Note: d.Hint, Source: "cis"}, stamp)
		}
	}
	cal.line("END:VCALENDAR")

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="naknak.ics"`)
	w.Header().Set("Cache-Control", "private, max-age=900")
	w.Write([]byte(cal.String()))
}

type agendaEvent struct {
	Date     string `json:"date"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Kind     string `json:"kind"`
	ModuleNr string `json:"module_nr"`
	Title    string `json:"title"`
	Lecturer string `json:"lecturer"`
	Room     string `json:"room"`
	Note     string `json:"note"`
	Source   string `json:"source"`
}

func (s *Server) writeEvent(cal *icsWriter, e agendaEvent, stamp string) {
	day := e.Date
	if i := strings.Index(day, " "); i >= 0 { // "Fr 02.10.2026"
		day = day[i+1:]
	}
	start, err := time.ParseInLocation("02.01.2006 15:04", day+" "+e.Start, s.app.Zone)
	if err != nil {
		return
	}
	end, err := time.ParseInLocation("02.01.2006 15:04", day+" "+e.End, s.app.Zone)
	if err != nil || end.Before(start) {
		end = start
	}
	summary := e.Title
	alarm := ""
	switch {
	case e.Kind == "Klausur":
		summary, alarm = "Klausur: "+e.Title, "-P1D"
	case strings.HasPrefix(e.Source, "moodle"):
		summary, alarm = "Fällig: "+e.Title, "-P1D"
	case e.Kind == "Frist":
		alarm = "-P2D"
	}
	sum := sha1.Sum([]byte(strings.Join([]string{e.Source, day, e.Start, e.Title}, "|")))
	cal.line("BEGIN:VEVENT")
	cal.prop("UID", hex.EncodeToString(sum[:10])+"@naknak")
	cal.line("DTSTAMP:" + stamp)
	cal.line("DTSTART:" + start.UTC().Format("20060102T150405Z"))
	cal.line("DTEND:" + end.UTC().Format("20060102T150405Z"))
	cal.prop("SUMMARY", summary)
	if e.Room != "" {
		cal.prop("LOCATION", e.Room)
	}
	var desc []string
	for _, x := range []string{e.Kind, e.ModuleNr, e.Lecturer, e.Note} {
		if x != "" {
			desc = append(desc, x)
		}
	}
	if len(desc) > 0 {
		cal.prop("DESCRIPTION", strings.Join(desc, "\n"))
	}
	if strings.HasPrefix(e.Source, "moodle") {
		cal.prop("CATEGORIES", "Moodle")
	} else {
		cal.prop("CATEGORIES", "CIS")
	}
	if alarm != "" {
		cal.line("BEGIN:VALARM")
		cal.line("ACTION:DISPLAY")
		cal.line("TRIGGER:" + alarm)
		cal.prop("DESCRIPTION", summary)
		cal.line("END:VALARM")
	}
	cal.line("END:VEVENT")
}

// icsWriter applies RFC 5545 escaping and folds lines at 75 octets without
// splitting UTF-8 characters.
type icsWriter struct{ b strings.Builder }

func (c *icsWriter) String() string { return c.b.String() }

func (c *icsWriter) prop(name, value string) {
	v := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(value)
	c.line(name + ":" + v)
}

func (c *icsWriter) line(s string) {
	for len(s) > 75 {
		cut := 75
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		c.b.WriteString(s[:cut] + "\r\n")
		s = " " + s[cut:]
	}
	c.b.WriteString(s + "\r\n")
}

// cachedTool reads a tool result the way the UI does: fresh cache, else stale
// cache (refreshed behind the answer), else a fetch.
func (s *Server) cachedTool(name string, args tools.Args) (json.RawMessage, error) {
	t := s.reg.Get(name)
	if t == nil {
		return nil, fmt.Errorf("unknown tool %s", name)
	}
	k, _ := json.Marshal(args)
	key := t.Name + " " + string(k)
	if e, ok := s.store.get(key); ok && usable(e) {
		age := s.cfg.Now().Sub(e.At)
		if age < s.freshFor(name) {
			s.stats.hit(name)
			return e.Res, nil
		}
		if age < maxStale {
			s.stats.hit(name)
			go s.refresh(t, key, args)
			return e.Res, nil
		}
	}
	res, _, err := s.refresh(t, key, args)
	return res, err
}
