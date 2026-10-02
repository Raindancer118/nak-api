package tools

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/moodle"
	"github.com/Raindancer118/nak-api/internal/planning"
	"github.com/Raindancer118/nak-api/internal/thesis"
	"github.com/Raindancer118/nak-api/internal/transfer"
)

const (
	thesisPath    = "/studium/bachelor/bachelorthesis"
	thesisApply   = "/studium/bachelor/bachelorthesis-alt/anmeldung-bachelorthesis"
	thesisReviews = "/studium/bachelor/bachelorthesis-alt/gutachtende"
)

var planningAidLink = regexp.MustCompile(`(?s)<a[^>]+href="([^"]+)"[^>]*>([^<]*Planungshilfe[^<]*)</a>`)

// thesisDates: lecture quarters and graduation deadlines, the calculator's input.
func thesisDates(a *app.App, c *client.Client) ([]thesis.Quarter, []thesis.Deadline) {
	var qs []thesis.Quarter
	if raw, err := planning.FetchVorlesungszeiten(c); err == nil {
		for _, q := range raw {
			f, err1 := time.ParseInLocation("02.01.2006", q.From, a.Zone)
			t, err2 := time.ParseInLocation("02.01.2006", q.To, a.Zone)
			if err1 == nil && err2 == nil {
				qs = append(qs, thesis.Quarter{Name: q.Name, From: f, To: t})
			}
		}
	}
	var ds []thesis.Deadline
	if raw, err := planning.FetchAbschlussfristen(c); err == nil {
		for _, f := range raw {
			lg, err1 := time.ParseInLocation("02.01.2006", f.LetzteNoten, a.Zone)
			b, err2 := time.ParseInLocation("02.01.2006", f.Pruefungsausschuss, a.Zone)
			if err1 == nil && err2 == nil {
				ds = append(ds, thesis.Deadline{Name: f.Abschluss, LastGrades: lg, Board: b, Ceremony: f.Graduierung})
			}
		}
	}
	return qs, ds
}

func planningAid(c *client.Client) thesis.PlanningAid {
	p, err := c.Page(thesisPath)
	if err != nil {
		return thesis.PlanningAid{}
	}
	m := planningAidLink.FindStringSubmatch(p.Body)
	if m == nil {
		return thesis.PlanningAid{}
	}
	url := html.UnescapeString(m[1])
	data, _, _, err := c.Download(url)
	if err != nil {
		return thesis.PlanningAid{}
	}
	text, err := moodle.PDFText(data)
	if err != nil {
		return thesis.PlanningAid{}
	}
	aid := thesis.ParsePlanningAid(text)
	aid.URL = url
	return aid
}

func thesisTools() []*Tool {
	return []*Tool{
		{Name: "nak_thesis", Kind: Read, Desc: "Bachelorthesis: ob die Anmeldung möglich ist (Begründung aus dem CIS), Termine aus der Planungshilfe des Jahrgangs, je Abschlusstermin die späteste Anmeldewoche und der späteste Start, Stand der Transferleistungen 1–6 und die Regeln mit Quellen.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				out := map[string]any{"rules": map[string]any{
					"months": thesis.Months, "extension_weeks": thesis.ExtensionWeeks, "review_lecture_weeks": thesis.ReviewWeeks,
					"start_after_weeks": thesis.StartAfterWeeks, "tl_weeks": thesis.TLWeeks,
					"sources": []string{"PO Wirtschaftsinformatik ab Jahrgang 23 § 7", "PVO vom 20.08.2026 § 17 Abs. 3, § 22", "CIS: Studium → Bachelor → Bachelorthesis (Terminplanung, Planungshilfe)"},
				}}
				if p, err := c.Page(thesisApply); err == nil {
					out["eligibility"] = thesis.ParseEligibility(p.Body)
				}
				aid := planningAid(c)
				out["planning_aid"] = aid
				qs, ds := thesisDates(a, c)
				now := a.Now().In(a.Zone)
				earliest, _ := time.ParseInLocation("02.01.2006", aid.EarliestRegistration, a.Zone)
				latest := []map[string]any{}
				for _, d := range ds {
					l := thesis.Latest(d, qs, ds)
					if !l.Found {
						continue
					}
					regTo, _ := time.ParseInLocation("02.01.2006", l.RegisterWeekTo, a.Zone)
					if regTo.Before(now) {
						continue // that registration week is over
					}
					// before the earliest registration of the cohort (end of semester 6)
					// this graduation cannot be reached
					tooEarly := !earliest.IsZero() && regTo.Before(earliest)
					latest = append(latest, map[string]any{"latest": l, "reachable": !tooEarly})
				}
				out["latest"] = latest
				if list, err := transfer.FetchList(c); err == nil {
					passed, t6 := 0, "offen"
					for _, r := range list {
						ok := strings.Contains(strings.ToLower(r.Wertung), "bestanden") && !strings.Contains(strings.ToLower(r.Wertung), "nicht")
						if n := strings.TrimSpace(r.No); len(n) == 1 && n >= "1" && n <= "5" && ok {
							passed++
						}
						if r.No == "6" {
							t6 = map[bool]string{true: "bestanden", false: "angemeldet"}[ok]
						}
					}
					out["transfer"] = map[string]any{"t1_5_passed": passed, "t6": t6}
				}
				return out, nil
			}},
		{Name: "nak_thesis_calc", Kind: Read, Desc: "Bachelorthesis-Rechner: aus Anmeldedatum oder Startdatum Beginn, Abgabe (2 Monate, Wochenende → Montag), Abgabe mit voller Verlängerung (5 Wochen), Ende der Begutachtung (4 Vorlesungswochen) und den erreichbaren Abschlusstermin.",
			Params: []Param{{Name: "start", Desc: "Beginn der Bearbeitungszeit, YYYY-MM-DD"}, {Name: "registration", Desc: "oder: Tag der Anmeldung, YYYY-MM-DD (Beginn = Montag zwei Wochen nach der Anmeldewoche)"}},
			Run: func(a *app.App, args Args) (any, error) {
				var start time.Time
				if s := args.Str("start"); s != "" {
					t, err := time.ParseInLocation("2006-01-02", s, a.Zone)
					if err != nil {
						return nil, err
					}
					start = t
				} else if s := args.Str("registration"); s != "" {
					t, err := time.ParseInLocation("2006-01-02", s, a.Zone)
					if err != nil {
						return nil, err
					}
					start = thesis.StartFor(t)
				} else {
					return nil, fmt.Errorf("start oder registration angeben (YYYY-MM-DD)")
				}
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				qs, ds := thesisDates(a, c)
				return thesis.Calc(start, qs, ds), nil
			}},
		{Name: "nak_thesis_match", Kind: Read, Desc: "Betreuungs-Assistent für die Bachelorthesis: zu einem Thema passende Gutachtende mit Match in Prozent, den passenden Fachgebieten und der Auslastung; mit reviewer zusätzlich, wie gut genau diese Person passt und wer besser passen würde. Läuft lokal, das Thema verlässt den Server nicht.",
			Params: []Param{{Name: "topic", Required: true, Desc: "Thema oder Arbeitstitel"}, {Name: "reviewer", Desc: "Name wie in der Übersicht, z.B. 'Bleek, Wolf-Gideon'"}},
			Run: func(a *app.App, args Args) (any, error) {
				topic := strings.TrimSpace(args.Str("topic"))
				if topic == "" {
					return nil, fmt.Errorf("topic fehlt")
				}
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				p, err := c.Page(thesisReviews)
				if err != nil {
					return nil, err
				}
				return matchResult(topic, args.Str("reviewer"), thesis.ParseReviewers(p.Body), embedderFor(a.ConfigDir))
			}},
		{Name: "nak_thesis_reviewers", Kind: Read, Desc: "Gutachtende für die Bachelorthesis: Name, Fachbereich, Fachgebiete und aktuelle Auslastung (frei/mittel/voll), optional gefiltert (z.B. 'datenbank').",
			Params: []Param{{Name: "query", Desc: "Suchwörter (alle müssen in Name, Fachbereich oder Fachgebiet vorkommen)"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				p, err := c.Page(thesisReviews)
				if err != nil {
					return nil, err
				}
				return thesis.FilterReviewers(thesis.ParseReviewers(p.Body), args.Str("query")), nil
			}},
	}
}

// applyThesisDeadline replaces the plan's estimate with the registration
// rules: the planning aid's latest registration week if the cohort has one,
// else the computed one for the next bachelor graduation.
func applyThesisDeadline(a *app.App, c *client.Client, plan *transfer.Plan) {
	qs, ds := thesisDates(a, c)
	now := a.Now().In(a.Zone)
	if aid := planningAid(c); aid.Found && aid.LatestRegisterTo != "" {
		if to, err := time.ParseInLocation("02.01.2006", aid.LatestRegisterTo, a.Zone); err == nil {
			plan.ThesisFrom = aid.EarliestStart
			plan.ThesisRegisterBy = to.AddDate(0, 0, -7*thesis.TLWeeks).Format("02.01.2006")
			plan.ThesisRegisterUntil = aid.LatestRegisterTo
			return
		}
	}
	for _, d := range ds {
		if d.LastGrades.Before(now) || !strings.Contains(d.Ceremony, "Bachelor") {
			continue
		}
		if l := thesis.Latest(d, qs, ds); l.Found {
			plan.ThesisRegisterBy, plan.ThesisRegisterUntil = l.TLStartBy, l.RegisterWeekTo
			return
		}
	}
}

// matchResult: suggestions, or one reviewer's fit plus better alternatives.
// emb (optional) adds the meaning of topic and subject areas to the words.
func matchResult(topic, reviewer string, rs []thesis.Reviewer, emb thesis.Embedder) (any, error) {
	all := thesis.ScoreSemantic(topic, rs, emb)
	semantic := all != nil
	suggest := func(n int) []thesis.Match {
		if semantic {
			return thesis.Rank(all, n)
		}
		return thesis.Suggest(topic, rs, n)
	}
	if strings.TrimSpace(reviewer) == "" {
		return map[string]any{"topic": topic, "suggestions": suggest(8), "semantic": semantic}, nil
	}
	for i, r := range rs {
		if strings.EqualFold(strings.TrimSpace(r.Name), strings.TrimSpace(reviewer)) {
			m := thesis.MatchAmong(topic, r, rs)
			if semantic {
				m = all[i]
			}
			var better []thesis.Match
			for _, s := range suggest(4) {
				if s.Reviewer.Name != r.Name && s.Percent > m.Percent {
					better = append(better, s)
				}
			}
			return map[string]any{"topic": topic, "match": m, "better": better, "semantic": semantic}, nil
		}
	}
	return nil, fmt.Errorf("keine gutachtende Person %q in der Übersicht", reviewer)
}

var embedders sync.Map // config dir → thesis.Embedder

// embedderFor: the local embedding model from NAK_EMBED_URL (Ollama) and
// NAK_EMBED_MODEL (default bge-m3), vectors cached in the config dir; nil
// when not configured.
func embedderFor(dir string) thesis.Embedder {
	url := strings.TrimSpace(os.Getenv("NAK_EMBED_URL"))
	if url == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv("NAK_EMBED_MODEL"))
	if model == "" {
		model = "bge-m3"
	}
	k := dir + "|" + url + "|" + model
	if e, ok := embedders.Load(k); ok {
		return e.(thesis.Embedder)
	}
	file := filepath.Join(dir, "embed-"+strings.NewReplacer("/", "_", ":", "_").Replace(model)+".json")
	e, _ := embedders.LoadOrStore(k, thesis.NewCachedEmbedder(thesis.NewOllama(url, model), file))
	return e.(thesis.Embedder)
}

// MatchResult is matchResult for other registries (demo).
func MatchResult(topic, reviewer string, rs []thesis.Reviewer) (any, error) {
	return matchResult(topic, reviewer, rs, nil)
}
