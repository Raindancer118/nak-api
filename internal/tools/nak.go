package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/exams"
	"github.com/Raindancer118/nak-api/internal/grades"
	"github.com/Raindancer118/nak-api/internal/health"
	"github.com/Raindancer118/nak-api/internal/planning"
	"github.com/Raindancer118/nak-api/internal/profile"
	"github.com/Raindancer118/nak-api/internal/transfer"
)

// Deadline is one entry of the merged CIS + Moodle deadline list.
type Deadline struct {
	When     string `json:"when"`
	Days     int    `json:"in_days"`
	What     string `json:"what"`
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	Course   string `json:"course,omitempty"`
	Hint     string `json:"hint,omitempty"`
	URL      string `json:"url,omitempty"`
	Urgent   bool   `json:"urgent,omitempty"`
	Overdue  bool   `json:"overdue,omitempty"`
	ExamID   string `json:"exam_id,omitempty"`
	ModuleNr string `json:"module_nr,omitempty"`
	at       time.Time
}

// partial collects results of independent sources; one failing system must
// not hide the other.
type partial struct {
	mu     sync.Mutex
	errors map[string]string
}

func (p *partial) fail(src string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.errors == nil {
		p.errors = map[string]string{}
	}
	p.errors[src] = err.Error()
}

func parseLocal(a *app.App, layout, s string) (time.Time, bool) {
	t, err := time.ParseInLocation(layout, s, a.Zone)
	return t, err == nil
}

func (a deadlineCtx) add(d Deadline) {
	a.mu.Lock()
	defer a.mu.Unlock()
	*a.out = append(*a.out, d)
}

type deadlineCtx struct {
	mu  *sync.Mutex
	out *[]Deadline
}

func collectDeadlines(a *app.App, days int) ([]Deadline, map[string]string) {
	now := a.Now().In(a.Zone)
	until := now.AddDate(0, 0, days)
	var out []Deadline
	var mu sync.Mutex
	ctx := deadlineCtx{&mu, &out}
	var p partial
	var wg sync.WaitGroup
	inRange := func(t time.Time) bool { return !t.Before(now.Add(-24*time.Hour)) && t.Before(until) }
	mk := func(t time.Time, what, kind, src string) Deadline {
		d := Deadline{When: dayLabel(t) + " " + t.Format("15:04"), What: what, Kind: kind, Source: src, at: t}
		d.Days = int(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.Zone).Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, a.Zone)).Hours() / 24)
		d.Overdue = t.Before(now)
		d.Urgent = !d.Overdue && t.Sub(now) < 72*time.Hour
		return d
	}

	wg.Add(1)
	go func() { // Moodle: calendar action events + assignments
		defer wg.Done()
		s, err := a.Moodle()
		if err != nil {
			p.fail("moodle", err)
			return
		}
		ev, err := s.Upcoming(int64(days), nil, 50)
		if err != nil {
			p.fail("moodle", err)
			return
		}
		for _, e := range ev {
			unix, _ := e.Get("due_unix").(int64)
			if unix == 0 {
				continue
			}
			t := time.Unix(unix, 0).In(a.Zone)
			what := fmt.Sprint(e.Get("activity"))
			if what == "<nil>" || what == "" {
				what = fmt.Sprint(e.Get("event"))
			}
			d := mk(t, what, "moodle_"+fmt.Sprint(e.Get("type")), "moodle")
			d.Course, _ = e.Get("course").(string)
			d.URL, _ = e.Get("url").(string)
			if act, ok := e.Get("action").(string); ok {
				d.Hint = act
			}
			ctx.add(d)
		}
	}()

	wg.Add(1)
	go func() { // CIS: exams, transfer, graduation deadlines
		defer wg.Done()
		c, err := a.CIS()
		if err != nil {
			p.fail("cis", err)
			return
		}
		if list, err := exams.FetchList(c); err != nil {
			p.fail("cis_exams", err)
		} else {
			for _, e := range list {
				name := strings.TrimSpace(e.ModuleNr + " " + e.Title)
				if start, ok := parseLocal(a, "02.01.2006 15:04", e.Start); ok && e.Registered && inRange(start) {
					d := mk(start, "Klausur "+name, "exam", "cis")
					d.ExamID, d.ModuleNr = e.ExamID, e.ModuleNr
					ctx.add(d)
				}
				if e.Deadlines == nil {
					continue
				}
				if e.Action == "register" {
					if t, ok := parseLocal(a, "02.01.2006 15:04", e.Deadlines.RegisterCloses); ok && inRange(t) {
						d := mk(t, "Anmeldeschluss "+name, "exam_registration", "cis")
						d.ExamID, d.ModuleNr, d.Hint = e.ExamID, e.ModuleNr, "nicht angemeldet — Anmeldung per cis_klausur_action möglich"
						ctx.add(d)
					}
				}
				if e.Registered {
					if t, ok := parseLocal(a, "02.01.2006 15:04", e.Deadlines.DeregisterUntil); ok && inRange(t) {
						d := mk(t, "Letzte Abmeldemöglichkeit "+name, "exam_deregistration", "cis")
						d.ExamID, d.ModuleNr = e.ExamID, e.ModuleNr
						ctx.add(d)
					}
				}
			}
		}
		if reps, err := transfer.FetchList(c); err != nil {
			p.fail("cis_transfer", err)
		} else {
			for _, r := range reps {
				if strings.Contains(strings.ToLower(r.Status), "bewertet") {
					continue
				}
				if t, ok := parseLocal(a, "02.01.2006", r.Abgabedatum); ok {
					t = t.Add(23*time.Hour + 59*time.Minute)
					if inRange(t) {
						ctx.add(mk(t, fmt.Sprintf("Abgabe Transferleistung %s: %s", r.No, r.Topic), "transfer", "cis"))
					}
				}
			}
		}
		if fs, err := planning.FetchAbschlussfristen(c); err == nil {
			for _, f := range fs {
				if t, ok := parseLocal(a, "02.01.2006", f.LetzteNoten); ok && inRange(t) {
					ctx.add(mk(t, "Letzte Noten für Abschluss "+f.Abschluss, "graduation", "cis"))
				}
			}
		}
	}()
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out, p.errors
}

var moduleNrRe = regexp.MustCompile(`\b([A-Z]{1,2}\d{3})\b`)

func nakTools() []*Tool {
	return []*Tool{
		{Name: "nak_deadlines", Kind: Read, Desc: "ALLE Fristen aus CIS und Moodle chronologisch: Moodle-Abgaben/Tests, Prüfungstermine, Anmeldeschluss offener Prüfungen, letzte Abmeldemöglichkeit, Transferleistungs-Abgaben, Abschlussfristen. Markiert dringende (<72 h).",
			Params: []Param{{Name: "days", Type: "integer", Desc: "Zeitraum in Tagen (Standard 30)"}},
			Run: func(a *app.App, args Args) (any, error) {
				days, err := args.Int("days", 30)
				if err != nil {
					return nil, err
				}
				ds, errs := collectDeadlines(a, int(days))
				res := map[string]any{"deadlines": ds, "count": len(ds), "generated": a.Now().In(a.Zone).Format("2006-01-02 15:04")}
				if len(errs) > 0 {
					res["unavailable_sources"] = errs
				}
				return res, nil
			}},
		{Name: "nak_agenda", Kind: Read, Desc: "Kombinierter Kalender für einen Zeitraum: Vorlesungen (Stundenplan, nur eigene WPF), Moodle-Termine/Fristen und eigene Klausuren — nach Tagen gruppiert.",
			Params: []Param{{Name: "from", Desc: "YYYY-MM-DD (Standard heute)"}, {Name: "days", Type: "integer", Desc: "Anzahl Tage (Standard 7)"}, {Name: "to", Desc: "YYYY-MM-DD"}},
			Run:    func(a *app.App, args Args) (any, error) { return agenda(a, args) }},
		{Name: "nak_dashboard", Kind: Read, Desc: "Der Überblick in einem Aufruf: heutige & morgige Vorlesungen, dringende Fristen (CIS + Moodle, 14 Tage), angemeldete Prüfungen, ungelesene Moodle-Nachrichten, aktuelles Quartal, Kopierguthaben. Guter Einstieg.",
			Run: func(a *app.App, args Args) (any, error) { return dashboard(a) }},
		{Name: "nak_selfcheck", Kind: Read, Desc: "Prüft (nur lesend) alle CIS-Seiten und -Formulare, auf die nak angewiesen ist, ob sie noch die erwartete Struktur haben. 'drift' = das CIS hat sich geändert (dann nak_report_drift vorschlagen); 'unavailable' = Netz/Login/Wartung.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := a.CIS()
				if err != nil {
					return nil, err
				}
				rep := health.Run(c, Version, a.Now())
				return selfcheckSummary(rep), nil
			}},
		{Name: "nak_report_drift", Kind: Write, Desc: "Meldet vom Self-Check gefundene CIS-Änderungen als GitHub-Issue (ein Issue pro Prüfung, nur Seitenstruktur, keine persönlichen Daten; schließt wieder grüne Prüfungen). Braucht NAK_GITHUB_TOKEN oder gh-Login; ohne Token gibt es vorbefüllte Links.",
			Preview: func(a *app.App, args Args) (any, error) {
				c, err := a.CIS()
				if err != nil {
					return nil, err
				}
				rep := health.Run(c, Version, a.Now())
				var issues []map[string]string
				for _, r := range rep.Drifted() {
					issues = append(issues, map[string]string{"title": health.Title(r), "body": health.Body(r, Version, rep.At), "manual_link": health.IssueURL(health.DefaultRepo, r, Version, rep.At)})
				}
				return map[string]any{"action": "GitHub-Issues in " + health.DefaultRepo + " anlegen/aktualisieren", "drifted": len(issues), "issues": issues,
					"token_available": health.Token() != ""}, nil
			},
			Do: func(a *app.App, args Args) (any, error) {
				c, err := a.CIS()
				if err != nil {
					return nil, err
				}
				rep := health.Run(c, Version, a.Now())
				tok := health.Token()
				if tok == "" {
					return nil, fmt.Errorf("no GitHub token (NAK_GITHUB_TOKEN or 'gh auth login') — use the manual links from the preview")
				}
				return (&health.GitHub{API: "https://api.github.com", Repo: health.DefaultRepo, Token: tok}).Sync(rep)
			}},
		{Name: "nak_module", Kind: Read, Desc: "Alles zu einem Modul über beide Systeme: CIS-Note/Status/Versuch, Prüfungstermin & Anmeldestatus, Studienplan (Semester, Prüfungsform, Credits), nächste Termine, Notenverteilung und der zugehörige Moodle-Kurs mit offenen Aufgaben.",
			Params: []Param{{Name: "module_nr", Required: true, Desc: "Modulnummer, z.B. I151"}},
			Run: func(a *app.App, args Args) (any, error) {
				return moduleOverview(a, strings.ToUpper(args.Str("module_nr")))
			}},
	}
}

func agenda(a *app.App, args Args) (any, error) {
	from, to, err := parseRange(a, args)
	if err != nil {
		return nil, err
	}
	var all []agendaEvent
	var p partial
	var notes []string
	if c, err := a.CIS(); err != nil {
		p.fail("cis", err)
	} else {
		evs, n, err := lectures(a, c, Args{}, from, to)
		if err != nil {
			p.fail("cis_stundenplan", err)
		}
		all, notes = append(all, evs...), n
		if list, err := exams.FetchList(c); err == nil {
			for _, e := range list {
				if t, ok := parseLocal(a, "02.01.2006 15:04", e.Start); ok && e.Registered && !t.Before(from) && t.Before(to) {
					end, _ := parseLocal(a, "02.01.2006 15:04", e.Ende)
					all = append(all, agendaEvent{Date: dayLabel(t), Start: t.Format("15:04"), End: end.Format("15:04"), Kind: "Klausur",
						ModuleNr: e.ModuleNr, Title: e.Title, Source: "cis_pruefung", sortKey: t})
				}
			}
		}
	}
	if s, err := a.Moodle(); err != nil {
		p.fail("moodle", err)
	} else {
		days := int64(to.Sub(a.Now()).Hours()/24) + 1
		if days > 0 {
			ev, err := s.Upcoming(days, nil, 50)
			if err != nil {
				p.fail("moodle", err)
			}
			for _, e := range ev {
				unix, _ := e.Get("due_unix").(int64)
				t := time.Unix(unix, 0).In(a.Zone)
				if unix == 0 || t.Before(from) || !t.Before(to) {
					continue
				}
				course, _ := e.Get("course").(string)
				all = append(all, agendaEvent{Date: dayLabel(t), Start: t.Format("15:04"), End: t.Format("15:04"), Kind: "Moodle-Frist",
					Title: fmt.Sprintf("%v (%s)", e.Get("event"), course), Source: "moodle", sortKey: t})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].sortKey.Before(all[j].sortKey) })
	type day struct {
		Date   string        `json:"date"`
		Events []agendaEvent `json:"events"`
	}
	var daysOut []*day
	idx := map[string]*day{}
	for _, e := range all {
		d := idx[e.Date]
		if d == nil {
			d = &day{Date: e.Date}
			idx[e.Date] = d
			daysOut = append(daysOut, d)
		}
		d.Events = append(d.Events, e)
	}
	res := map[string]any{"from": from.Format("2006-01-02"), "to": to.AddDate(0, 0, -1).Format("2006-01-02"), "days": daysOut, "notes": notes}
	if len(p.errors) > 0 {
		res["unavailable_sources"] = p.errors
	}
	return res, nil
}

func dashboard(a *app.App) (any, error) {
	now := a.Now().In(a.Zone)
	res := map[string]any{"now": dayLabel(now) + " " + now.Format("15:04")}
	var p partial
	var wg sync.WaitGroup
	var mu sync.Mutex
	set := func(k string, v any) {
		mu.Lock()
		res[k] = v
		mu.Unlock()
	}
	wg.Add(3)
	go func() {
		defer wg.Done()
		ds, errs := collectDeadlines(a, 14)
		var urgent, rest []Deadline
		for _, d := range ds {
			if d.Urgent || d.Overdue {
				urgent = append(urgent, d)
			} else {
				rest = append(rest, d)
			}
		}
		set("deadlines_urgent", urgent)
		if len(rest) > 8 {
			rest = rest[:8]
		}
		set("deadlines_next", rest)
		for k, v := range errs {
			p.fail(k, fmt.Errorf("%s", v))
		}
	}()
	go func() {
		defer wg.Done()
		c, err := a.CIS()
		if err != nil {
			p.fail("cis", err)
			return
		}
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, a.Zone)
		if evs, _, err := lectures(a, c, Args{}, today, today.AddDate(0, 0, 2)); err != nil {
			p.fail("cis_stundenplan", err)
		} else {
			set("lectures_today_tomorrow", evs)
		}
		if list, err := exams.FetchList(c); err == nil {
			var reg []map[string]any
			for _, e := range list {
				if e.Registered {
					reg = append(reg, map[string]any{"module": e.ModuleNr + " " + e.Title, "start": e.Start, "exam_id": e.ExamID})
				}
			}
			set("registered_exams", reg)
		}
		if qs, err := planning.FetchVorlesungszeiten(c); err == nil {
			cur, next := planning.Current(qs, now)
			set("quarter", map[string]any{"current": cur, "next": next})
		}
		if b, err := profile.FetchBalance(c); err == nil {
			set("balance", b)
		}
	}()
	go func() {
		defer wg.Done()
		s, err := a.Moodle()
		if err != nil {
			p.fail("moodle", err)
			return
		}
		d, err := s.Dashboard()
		if err != nil {
			p.fail("moodle", err)
			return
		}
		set("moodle_unread", map[string]any{"notifications": d.Get("unread_notifications"), "conversations": d.Get("unread_conversations")})
	}()
	wg.Wait()
	if len(p.errors) > 0 {
		res["unavailable_sources"] = p.errors
	}
	return res, nil
}

func moduleOverview(a *app.App, nr string) (any, error) {
	if !moduleNrRe.MatchString(nr) {
		return nil, fmt.Errorf("module_nr looks wrong: %q (expected e.g. I151)", nr)
	}
	res := map[string]any{"module_nr": nr}
	var p partial
	if c, err := a.CIS(); err != nil {
		p.fail("cis", err)
	} else {
		if o, err := grades.Fetch(c); err == nil {
			if m := o.Find(nr); m != nil {
				res["title"] = m.Title
				res["grade"] = m
				if m.StatisticURL != "" {
					if d, err := grades.FetchDistribution(c, nr); err == nil {
						res["distribution"] = d
					}
				}
			}
		} else {
			p.fail("cis_grades", err)
		}
		if sp, err := planning.FetchStudienplan(c); err == nil {
			for _, m := range sp.Modules {
				if m.ModuleNr == nr {
					res["studienplan"] = m
					if res["title"] == nil {
						res["title"] = m.Title
					}
				}
			}
		}
		if list, err := exams.FetchList(c); err == nil {
			var ex []exams.Exam
			for _, e := range list {
				if strings.Contains(","+e.ModuleNr+",", ","+nr+",") || strings.Contains(e.ModuleNr, nr) {
					ex = append(ex, e)
				}
			}
			res["exams"] = ex
		}
		now := a.Now().In(a.Zone)
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, a.Zone)
		if evs, _, err := lectures(a, c, Args{"module_nr": nr}, from, from.AddDate(0, 0, 60)); err == nil {
			if len(evs) > 10 {
				evs = evs[:10]
			}
			res["next_sessions"] = evs
		}
	}
	if s, err := a.Moodle(); err != nil {
		p.fail("moodle", err)
	} else if cs, err := s.Courses(nr, "all"); err != nil {
		p.fail("moodle", err)
	} else {
		res["moodle_courses"] = cs
		var open []any
		for _, c := range cs {
			id, _ := c.Get("id").(int64)
			if c.Get("state") != "current" || id == 0 {
				continue
			}
			if as, err := s.Assignments(&id, true); err == nil {
				for _, x := range as {
					open = append(open, x)
				}
			}
		}
		if len(open) > 0 {
			res["moodle_open_assignments"] = open
		}
	}
	if len(p.errors) > 0 {
		res["unavailable_sources"] = p.errors
	}
	if len(res) <= 1 {
		return nil, fmt.Errorf("module %s not found in CIS or Moodle", nr)
	}
	return res, nil
}

// Version is set by cmd at start-up.
var Version = "dev"

func selfcheckSummary(rep *health.Report) map[string]any {
	counts := map[string]int{}
	var problems []health.Result
	for _, r := range rep.Results {
		counts[r.Status]++
		if r.Status != health.StatusOK {
			problems = append(problems, r)
		}
	}
	return map[string]any{"version": rep.Version, "checks": len(rep.Results), "counts": counts, "problems": problems}
}

// All returns the complete tool set.
func All() *Registry {
	r := NewRegistry()
	r.Add(cisTools()...)
	r.Add(moodleTools()...)
	r.Add(nakTools()...)
	r.Add(eduvaultTools()...)
	return r
}
