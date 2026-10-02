package tools

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/auth"
	"github.com/Raindancer118/nak-api/internal/certs"
	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/exams"
	"github.com/Raindancer118/nak-api/internal/forms"
	"github.com/Raindancer118/nak-api/internal/grades"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"github.com/Raindancer118/nak-api/internal/moodle"
	"github.com/Raindancer118/nak-api/internal/pages"
	"github.com/Raindancer118/nak-api/internal/planning"
	"github.com/Raindancer118/nak-api/internal/profile"
	"github.com/Raindancer118/nak-api/internal/seminars"
	"github.com/Raindancer118/nak-api/internal/stundenplan"
	"github.com/Raindancer118/nak-api/internal/timetable"
	"github.com/Raindancer118/nak-api/internal/transfer"
	"github.com/Raindancer118/nak-api/internal/wahlpflicht"
)

func cis(a *app.App) (*client.Client, error) { return a.CIS() }

// saveFile writes data below the download dir (or to out) and never overwrites.
func saveFile(a *app.App, data []byte, name, out string) (map[string]any, error) {
	var target string
	if out != "" {
		target = a.ExpandPath(out)
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			target = moodle.FreeName(target, name)
		} else {
			target = moodle.FreeName(filepath.Dir(target), filepath.Base(target))
		}
	} else {
		target = moodle.FreeName(a.DownloadDir, name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return nil, err
	}
	return map[string]any{"path": target, "bytes": len(data)}, nil
}

func fileNameFromURL(u string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return "download"
	}
	return moodle.FileName(pu)
}

func submissionPreview(s *forms.Submission, what string) map[string]any {
	return map[string]any{"action": what, "request": s.Preview(), "changes": s.Changes}
}

func sendSubmission(c *client.Client, s *forms.Submission) (map[string]any, error) {
	p, err := s.Send(c)
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": p.Status, "message": exams.FlashMessage(p.Body)}, nil
}

func cisTools() []*Tool {
	return []*Tool{
		// ── session ──
		{Name: "cis_login", Kind: Local, Desc: "Am CIS anmelden (Session wird lokal gespeichert). Ohne Parameter mit CIS_USER/CIS_PASS aus der Umgebung.",
			Params: []Param{{Name: "username", Desc: "CIS-Login (Matrikel-/Loginnummer)"}, {Name: "password", Desc: "Passwort"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := a.CISClient()
				if err != nil {
					return nil, err
				}
				u, p := args.Str("username"), args.Str("password")
				if u == "" || p == "" {
					if c.Relogin == nil {
						if _, err := a.CIS(); err != nil {
							return nil, err
						}
						return "Logged in (environment credentials).", nil
					}
					if err := c.Relogin(c); err != nil {
						return nil, err
					}
					return "Logged in (environment credentials).", nil
				}
				if err := auth.Login(c, u, p); err != nil {
					return nil, err
				}
				return "Logged in. Session saved.", nil
			}},
		{Name: "cis_logout", Kind: Local, Desc: "Vom CIS abmelden und lokale Session löschen.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := a.CISClient()
				if err != nil {
					return nil, err
				}
				return "Logged out.", auth.Logout(c)
			}},
		{Name: "cis_status", Kind: Read, Desc: "Wer bin ich im CIS? Name, Login, Matrikel, Zenturie, Studiengang, Firma. Prüft dabei die Session.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				p, err := profile.FetchPersonal(c)
				if err != nil {
					return nil, err
				}
				return map[string]any{"name": p.Vorname + " " + p.Nachname, "login": p.Login, "matrikelnr": p.Matrikelnr,
					"zenturie": p.Zenturie, "jahrgang": p.Jahrgang(), "studiengang": p.Studiengang, "firma": p.Firma, "read_only_mode": a.ReadOnly}, nil
			}},

		// ── profile (read) ──
		{Name: "cis_profile", Kind: Read, Desc: "Persönliche Stammdaten aus 'Meine Daten' (Adresse, Telefon, E-Mails, Geburtsdatum, Firma, lebenslange E-Mail).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return profile.FetchPersonal(c)
			}},
		{Name: "cis_contact", Kind: Read, Desc: "Hinterlegte private E-Mail, Infopost-Einwilligung und Telefonnummern (Festnetz, Mobil, Fax).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return profile.FetchContact(c)
			}},
		{Name: "cis_address", Kind: Read, Desc: "Hinterlegte Adresse (Haupt- oder Semesteradresse) und wie sie in Briefen erscheint.",
			Params: []Param{{Name: "type", Desc: "Adresstyp-ID, z.B. 1=Hauptadresse, 5=Semesteradresse (Standard: Hauptadresse)"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return profile.FetchAddress(c, args.Str("type"))
			}},
		{Name: "cis_sharing", Kind: Read, Desc: "Freigaben: Noten/Prüfungsanmeldungen für den Ausbildungsbetrieb und Sichtbarkeit der eigenen Daten für Kommiliton:innen.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				f, err := profile.FetchFreigabe(c)
				if err != nil {
					return nil, err
				}
				d, err := profile.FetchDatenfreigabe(c)
				if err != nil {
					return nil, err
				}
				return map[string]any{"betrieb": f, "kommilitonen": d, "levels": profile.SharingLevels}, nil
			}},
		{Name: "cis_payment", Kind: Read, Desc: "Status des SEPA-Lastschriftmandats (IBAN maskiert). Bankdaten werden bewusst nicht geändert.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return profile.FetchPayment(c)
			}},
		{Name: "cis_classmates", Kind: Read, Desc: "Kommiliton:innen einer Zenturie (nur Name und Betrieb; Kontaktdaten Dritter werden aus Datenschutzgründen nicht ausgegeben).",
			Params: []Param{{Name: "zenturie", Desc: "z.B. I23a (Standard: eigene)"}, {Name: "list_zenturien", Type: "boolean", Desc: "Auch alle wählbaren Zenturien ausgeben"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				cm, err := profile.FetchClassmates(c, args.Str("zenturie"))
				if err != nil {
					return nil, err
				}
				if !args.Bool("list_zenturien", false) {
					cm.Zenturien = nil
				}
				return cm, nil
			}},
		{Name: "cis_balance", Kind: Read, Desc: "Kopierguthaben (Postfach > Guthaben).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return profile.FetchBalance(c)
			}},
		{Name: "cis_vertiefung", Kind: Read, Desc: "Gewählte Vertiefungsrichtung und wählbare Optionen (Jahrgänge bis 2023).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return profile.FetchVertiefung(c)
			}},

		// ── grades ──
		{Name: "cis_grades", Kind: Read, Desc: "Leistungsübersicht: alle Module mit Note, Versuch, Status, Credits, dazu Seminare, Transferleistungen, Durchschnitt, Credits gesamt und eine Auswertung.",
			Params: []Param{{Name: "status", Desc: "Filter Module: bestanden | nicht bestanden | offen", Enum: []string{grades.StatusPassed, grades.StatusFailed, grades.StatusOpen}}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				o, err := grades.Fetch(c)
				if err != nil {
					return nil, err
				}
				st := o.Stats()
				if f := args.Str("status"); f != "" {
					var keep []grades.ModuleGrade
					for _, m := range o.Modules {
						if m.Status == f {
							keep = append(keep, m)
						}
					}
					o.Modules = keep
				}
				return map[string]any{"overview": o, "stats": st}, nil
			}},
		{Name: "cis_grade_distribution", Kind: Read, Desc: "Notenverteilung einer Prüfung (Notenspiegel aller Teilnehmenden) inkl. Durchschnitt, Durchfallquote und der eigenen Platzierung.",
			Params: []Param{{Name: "module_nr", Required: true, Desc: "Modulnummer, z.B. I140"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return grades.FetchDistribution(c, args.Str("module_nr"))
			}},
		{Name: "cis_attendance", Kind: Read, Desc: "Anwesenheiten eines Moduls (jeder Termin mit Status) oder mit all_open=true eine Übersicht über alle noch offenen Module.",
			Params: []Param{{Name: "module_nr", Desc: "Modulnummer, z.B. I151"}, {Name: "all_open", Type: "boolean", Desc: "Übersicht aller offenen Module"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				if nr := args.Str("module_nr"); nr != "" {
					return grades.FetchAttendance(c, nr)
				}
				if !args.Bool("all_open", false) {
					return nil, fmt.Errorf("module_nr or all_open=true is required")
				}
				o, err := grades.Fetch(c)
				if err != nil {
					return nil, err
				}
				var out []map[string]any
				for _, m := range o.Modules {
					if m.Status != grades.StatusOpen || m.AttendanceURL == "" {
						continue
					}
					p, err := c.Page(m.AttendanceURL)
					if err != nil {
						return nil, err
					}
					att := grades.ParseAttendance(p.Body)
					if len(att.Sessions) == 0 {
						continue
					}
					out = append(out, map[string]any{"module_nr": m.ModuleNr, "module": m.Title, "sessions": len(att.Sessions),
						"summary": att.Summary, "attendance_rate_percent": att.AttendanceRate})
				}
				return out, nil
			}},
		{Name: "cis_transcript", Kind: Local, Desc: "Notenspiegel/Transcript als PDF herunterladen (deutsch oder englisch).",
			Params: []Param{{Name: "lang", Desc: "de (Standard) oder en", Enum: []string{"de", "en"}}, {Name: "output_path", Desc: "Zielpfad/-ordner (Standard: Download-Ordner)"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				lang := args.Str("lang")
				if lang == "" {
					lang = "de"
				}
				data, _, err := grades.FetchTranscript(c, lang)
				if err != nil {
					return nil, err
				}
				return saveFile(a, data, "Notenspiegel_"+lang+".pdf", args.Str("output_path"))
			}},

		// ── planning ──
		{Name: "cis_studienplan", Kind: Read, Desc: "Persönlicher Studienplan: Module je Semester mit Stunden, Prüfungssemester, Prüfungsform und Credits.",
			Params: []Param{{Name: "semester", Type: "integer", Desc: "Nur Module mit Last oder Prüfung in diesem Semester"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				sp, err := planning.FetchStudienplan(c)
				if err != nil {
					return nil, err
				}
				if s, _ := args.Int("semester", 0); s > 0 {
					return map[string]any{"semester": s, "modules": sp.BySemester(int(s)), "legend": sp.Legend}, nil
				}
				return map[string]any{"modules": sp.Modules, "legend": sp.Legend, "total_credits": sp.TotalCredits()}, nil
			}},
		{Name: "cis_progress", Kind: Read, Desc: "Studienfortschritt: Studienplan × Noten — bestandene/offene/nicht bestandene Module je Semester, Credits und Voraussetzungen für die Bachelorarbeit.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return progress(c)
			}},
		{Name: "cis_timetable", Kind: Read, Desc: "Stundenplan als Termine (aus dem Zenturie-Kalender): Vorlesungen, WPF, Klausuren mit Zeit, Dozent, Raum. Standard: die nächsten 7 Tage, nur eigene WPF.",
			Params: []Param{{Name: "from", Desc: "Startdatum YYYY-MM-DD (Standard heute)"}, {Name: "days", Type: "integer", Desc: "Anzahl Tage (Standard 7)"},
				{Name: "to", Desc: "Enddatum YYYY-MM-DD (statt days)"}, {Name: "zenturie", Desc: "z.B. I23a (Standard: eigene)"},
				{Name: "module_nr", Desc: "Nur dieses Modul"}, {Name: "kind", Desc: "Nur diese Art (V, WP, Z, …)"}, {Name: "text", Desc: "Volltextfilter (Titel, Dozent, Raum)"},
				{Name: "all_wpf", Type: "boolean", Desc: "true = alle WPF-Termine der Zenturie statt nur der eigenen"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return timetableTool(a, c, args)
			}},
		{Name: "cis_vorlesungszeiten", Kind: Read, Desc: "Vorlesungszeiten (Quartale) mit aktuellem und nächstem Quartal.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				qs, err := planning.FetchVorlesungszeiten(c)
				if err != nil {
					return nil, err
				}
				cur, next := planning.Current(qs, a.Now().In(a.Zone))
				return map[string]any{"current": cur, "next": next, "quarters": qs}, nil
			}},
		{Name: "cis_abschlussfristen", Kind: Read, Desc: "Abschlussfristen & Entlassungstermine (letzte Noten, Prüfungsausschuss, Graduierung).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return planning.FetchAbschlussfristen(c)
			}},
		{Name: "cis_list_stundenplan", Kind: Read, Desc: "Downloadbare Stundenplan-Dateien (.ics/.html) je Zenturie.",
			Params: []Param{{Name: "zenturie", Desc: "Präfix, z.B. I24a"}, {Name: "format", Desc: "ics oder html"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				all, err := stundenplan.FetchList(c)
				if err != nil {
					return nil, err
				}
				return stundenplan.Filter(all, args.Str("zenturie"), args.Str("format")), nil
			}},
		{Name: "cis_download_stundenplan", Kind: Local, Desc: "Stundenplan-Datei herunterladen (URL aus cis_list_stundenplan).",
			Params: []Param{{Name: "download_url", Required: true, Desc: "URL aus cis_list_stundenplan"}, {Name: "output_path", Desc: "Zielpfad/-ordner"}},
			Run: func(a *app.App, args Args) (any, error) {
				return downloadCIS(a, args.Str("download_url"), args.Str("output_path"))
			}},

		// ── exams ──
		{Name: "cis_list_klausuren", Kind: Read, Desc: "Prüfungsübersicht: alle Prüfungen mit examId, Termin, Status (angemeldet?), angebotener Aktion und berechneten An-/Abmeldefristen.",
			Params: []Param{{Name: "filter", Desc: "all (Standard) | registered | open (Anmeldung möglich)", Enum: []string{"all", "registered", "open"}}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				list, err := exams.FetchList(c)
				if err != nil {
					return nil, err
				}
				var out []exams.Exam
				for _, e := range list {
					switch args.Str("filter") {
					case "registered":
						if !e.Registered {
							continue
						}
					case "open":
						if e.Action != "register" {
							continue
						}
					}
					out = append(out, e)
				}
				return out, nil
			}},
		{Name: "cis_klausur_action", Kind: Write, Desc: "Zu einer Prüfung an- oder abmelden.",
			Params: []Param{{Name: "exam_id", Required: true, Desc: "examId aus cis_list_klausuren"},
				{Name: "action", Required: true, Desc: "register oder deregister — muss zur aktuell angebotenen Aktion passen", Enum: []string{"register", "deregister"}}},
			Preview: func(a *app.App, args Args) (any, error) {
				_, e, err := resolveExam(a, args)
				if err != nil {
					return nil, err
				}
				return map[string]any{"action": map[string]string{"register": "Prüfungsanmeldung", "deregister": "Prüfungsabmeldung"}[e.Action], "exam": e}, nil
			},
			Do: func(a *app.App, args Args) (any, error) {
				c, e, err := resolveExam(a, args)
				if err != nil {
					return nil, err
				}
				msg, err := exams.Submit(c, e.ActionURL)
				if err != nil {
					return nil, err
				}
				return map[string]any{"exam": e.ModuleNr + " " + e.Title, "message": msg}, nil
			}},

		// ── seminars ──
		{Name: "cis_list_seminars", Kind: Read, Desc: "Seminarprogramm eines Quartals (Titel, Dozent, Zeitraum, Kategorie, Status, abgesagt, mögliche Aktionen) oder 'Meine Seminare'.",
			Params: []Param{{Name: "quarter", Desc: "z.B. '2026 - 4' (Standard: aktuelles)"}, {Name: "mine", Type: "boolean", Desc: "true = Meine Seminare (alle Quartale)"},
				{Name: "category", Desc: "Filter auf Kategorie (Teilwort)"}, {Name: "available_only", Type: "boolean", Desc: "Nur Seminare mit Anmeldemöglichkeit"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				l, err := seminars.Fetch(c, args.Str("quarter"), args.Bool("mine", false))
				if err != nil {
					return nil, err
				}
				cat := strings.ToLower(args.Str("category"))
				var keep []seminars.Seminar
				for _, s := range l.Seminars {
					if cat != "" && !strings.Contains(strings.ToLower(s.Category), cat) {
						continue
					}
					if args.Bool("available_only", false) && (len(s.Actions) == 0 || s.Cancelled) {
						continue
					}
					keep = append(keep, s)
				}
				l.Seminars = keep
				return l, nil
			}},
		{Name: "cis_seminar_detail", Kind: Read, Desc: "Details eines Seminars: Termine, Bemerkung, Themenbereich, Prüfungsform, Kreditpunkte, Workload, Dozent, Inhalt.",
			Params: []Param{{Name: "seminar_id", Required: true, Desc: "id aus cis_list_seminars"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return seminars.FetchDetail(c, args.Str("seminar_id"))
			}},
		{Name: "cis_seminar_participation", Kind: Read, Desc: "Belegung eines Seminars: Anzahl Teilnehmende/Warteliste (Mindestteilnehmerzahl 16) und ob/wo man selbst steht. Keine Namen Dritter.",
			Params: []Param{{Name: "seminar_id", Required: true, Desc: "id aus cis_list_seminars"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				p, err := profile.FetchPersonal(c)
				if err != nil {
					return nil, err
				}
				return seminars.FetchParticipation(c, args.Str("seminar_id"), p.Nachname, p.Vorname)
			}},
		{Name: "cis_seminar_action", Kind: Write, Desc: "Seminaraktion ausführen: anmelden (subscribeToSeminar) oder auf die Warteliste (anWartelisteAnmelden) bzw. jede andere vom CIS angebotene Aktion. Teilnahme ist danach verpflichtend.",
			Params: []Param{{Name: "seminar_id", Required: true, Desc: "id aus cis_list_seminars"}, {Name: "action", Desc: "TYPO3-Aktion; leer = die einzige angebotene"}},
			Preview: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				s, act, err := seminars.ResolveAction(c, args.Str("seminar_id"), args.Str("action"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"action": act.Label, "typo3_action": act.Action, "seminar": s.Title, "from": s.From, "to": s.To, "info": s.Info,
					"hinweis": "Die Teilnahme an zugeteilten Seminaren ist verpflichtend; Abmeldung nur mit Attest, Arbeitgeberbestätigung oder Ersatzperson."}, nil
			},
			Do: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				s, act, err := seminars.ResolveAction(c, args.Str("seminar_id"), args.Str("action"))
				if err != nil {
					return nil, err
				}
				p, err := seminars.Submit(c, act)
				if err != nil {
					return nil, err
				}
				return map[string]any{"seminar": s.Title, "action": act.Label, "message": exams.FlashMessage(p.Body)}, nil
			}},

		// ── Wahlpflicht ──
		{Name: "cis_list_wahlpflicht", Kind: Read, Desc: "Wahlpflichtmodule: bereits gewählte und (im Wahlzeitraum) wählbare Module mit Dozent, Vertiefungsrichtung, Termin.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return wahlpflicht.Fetch(c)
			}},
		{Name: "cis_wahlpflicht_detail", Kind: Read, Desc: "Details eines Wahlpflichtmoduls (Inhalte, Lernziele, Literatur, Prüfungsform, Credits, Dozent) und wählbare Termine.",
			Params: []Param{{Name: "module_id", Required: true, Desc: "id aus cis_list_wahlpflicht"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return wahlpflicht.FetchDetail(c, args.Str("module_id"))
			}},
		{Name: "cis_select_wahlpflicht", Kind: Write, Desc: "Wahlpflichtmodul verbindlich wählen (nur im Wahlzeitraum).",
			Params: []Param{{Name: "module_id", Required: true, Desc: "id aus cis_list_wahlpflicht"}, {Name: "termin_id", Desc: "Termin-id aus cis_wahlpflicht_detail (nötig bei mehreren Terminen)"}},
			Preview: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				ch, err := wahlpflicht.PrepareSelect(c, args.Str("module_id"), args.Str("termin_id"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"action": "Wahlpflichtmodul wählen", "module": ch.Module, "termin": ch.Termin}, nil
			},
			Do: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				ch, err := wahlpflicht.PrepareSelect(c, args.Str("module_id"), args.Str("termin_id"))
				if err != nil {
					return nil, err
				}
				p, err := wahlpflicht.Submit(c, ch)
				if err != nil {
					return nil, err
				}
				return map[string]any{"module": ch.Module, "termin": ch.Termin.Text, "message": exams.FlashMessage(p.Body)}, nil
			}},

		// ── Transferleistungen ──
		{Name: "cis_list_transfer", Kind: Read, Desc: "Transferleistungen: Nr., Abgabedatum, Korrekturfrist, Thema, Modul, Wertung, Versuch, Status, Aktionen.",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return transfer.FetchList(c)
			}},
		{Name: "cis_transfer_bewertung", Kind: Read, Desc: "Bewertung einer Transferleistung: Kriterien mit Note, Gewichtung, Feedback, Dokumente und berechnete gewichtete Gesamtnote.",
			Params: []Param{{Name: "transfer_id", Required: true, Desc: "id aus cis_list_transfer"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return transfer.FetchBewertung(c, args.Str("transfer_id"))
			}},
		{Name: "cis_download_transfer_document", Kind: Local, Desc: "Dokument einer Transferleistung herunterladen (URL aus cis_transfer_bewertung).",
			Params: []Param{{Name: "download_url", Required: true, Desc: "Dokument-URL"}, {Name: "output_path", Desc: "Zielpfad/-ordner"}},
			Run: func(a *app.App, args Args) (any, error) {
				return downloadCIS(a, args.Str("download_url"), args.Str("output_path"))
			}},
		{Name: "cis_transfer_options", Kind: Read, Desc: "Was das Anmeldeformular für eine neue Transferleistung gerade anbietet (Bericht-Nr., Module, Sprachen).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return transfer.FetchNewFormOptions(c)
			}},
		{Name: "cis_transfer_register", Kind: Write, Desc: "Neue Transferleistung beantragen (Thema + Modul + Auftragsklärung als PDF).",
			Params: []Param{{Name: "no", Required: true, Desc: "Bericht-Nr. (aus cis_transfer_options)"}, {Name: "topic", Required: true, Desc: "Thema"},
				{Name: "module", Required: true, Desc: "Modul: moduleId oder Modulnummer wie I162"}, {Name: "language", Desc: "de (Standard) oder en"},
				{Name: "locked", Type: "boolean", Desc: "Sperrvermerk (Standard laut Formular)"}, {Name: "file_path", Required: true, Desc: "Lokaler Pfad zur Auftragsklärung (PDF, max. 10 MB)"}},
			Preview: func(a *app.App, args Args) (any, error) {
				_, s, err := prepareTransfer(a, args)
				if err != nil {
					return nil, err
				}
				return submissionPreview(s, "Transferleistung beantragen (2 Schritte: confirmNew → create)"), nil
			},
			Do: func(a *app.App, args Args) (any, error) {
				c, s, err := prepareTransfer(a, args)
				if err != nil {
					return nil, err
				}
				msg, err := transfer.Register(c, s)
				if err != nil {
					return nil, err
				}
				return map[string]any{"message": msg}, nil
			}},

		// ── profile (write) ──
		{Name: "cis_update_contact", Kind: Write, Desc: "Private E-Mail, Infopost-Einwilligung und/oder Telefonnummern im CIS ändern.",
			Params: []Param{{Name: "email", Desc: "Neue private E-Mail"}, {Name: "infopost", Type: "boolean", Desc: "Infopost nach dem Studium"},
				{Name: "phone", Desc: "Festnetz (vorwahl-rufnummer, leer = löschen)"}, {Name: "mobile", Desc: "Mobil"}, {Name: "fax", Desc: "Fax"}},
			Preview: func(a *app.App, args Args) (any, error) { return contactUpdate(a, args, false) },
			Do:      func(a *app.App, args Args) (any, error) { return contactUpdate(a, args, true) }},
		{Name: "cis_update_address", Kind: Write, Desc: "Adresse (Haupt- oder Semesteradresse) ändern oder löschen.",
			Params: []Param{{Name: "type", Desc: "Adresstyp-ID (Standard Hauptadresse; 5=Semesteradresse)"}, {Name: "zusatz", Desc: "Adresszusatz"},
				{Name: "strasse", Desc: "Straße + Nr."}, {Name: "plz", Desc: "PLZ"}, {Name: "ort", Desc: "Ort"}, {Name: "delete", Type: "boolean", Desc: "Adresse dieses Typs löschen"}},
			Preview: func(a *app.App, args Args) (any, error) { return addressUpdate(a, args, false) },
			Do:      func(a *app.App, args Args) (any, error) { return addressUpdate(a, args, true) }},
		{Name: "cis_set_sharing", Kind: Write, Desc: "Freigaben ändern: Noten/Prüfungsanmeldungen für den Betrieb und/oder Sichtbarkeit für Kommiliton:innen.",
			Params: []Param{{Name: "noten_betrieb", Type: "boolean", Desc: "Noten für den Ausbildungsbetrieb freigeben"},
				{Name: "anmeldungen_betrieb", Type: "boolean", Desc: "Prüfungs-/Transferanmeldungen für den Betrieb freigeben"},
				{Name: "kommilitonen", Type: "object", Desc: "Sichtbarkeit je Feld: {\"Adresse\":\"0\",\"Mobil\":\"1\"}; Felder Name, Firma, Geburtsdatum, Adresse, Fest, Mobil, Mail; 0=nicht, 1=Zenturie, 2=alle"}},
			Preview: func(a *app.App, args Args) (any, error) { return sharingUpdate(a, args, false) },
			Do:      func(a *app.App, args Args) (any, error) { return sharingUpdate(a, args, true) }},
		{Name: "cis_set_vertiefung", Kind: Write, Desc: "Vertiefungsrichtung beantragen (Jahrgänge bis 2023).",
			Params: []Param{{Name: "value", Required: true, Desc: "Options-Wert aus cis_vertiefung (z.B. 8)"}},
			Preview: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				s, err := profile.PrepareVertiefung(c, args.Str("value"))
				if err != nil {
					return nil, err
				}
				return submissionPreview(s, "Vertiefungsrichtung beantragen"), nil
			},
			Do: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				s, err := profile.PrepareVertiefung(c, args.Str("value"))
				if err != nil {
					return nil, err
				}
				return sendSubmission(c, s)
			}},

		// ── certificates & documents ──
		{Name: "cis_list_certs", Kind: Read, Desc: "Online-Studienbescheinigungen je Semester (de/en).",
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				return certs.FetchList(c)
			}},
		{Name: "cis_download_cert", Kind: Local, Desc: "Studienbescheinigung als PDF herunterladen — per URL oder per Semester (z.B. 'WS 2026', leer = neuestes) und Sprache.",
			Params: []Param{{Name: "download_url", Desc: "URL aus cis_list_certs"}, {Name: "semester", Desc: "z.B. 'WS 2026' oder 'SS 2026'"},
				{Name: "lang", Desc: "de (Standard) oder en"}, {Name: "output_path", Desc: "Zielpfad/-ordner"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				u, name := args.Str("download_url"), ""
				if u == "" {
					list, err := certs.FetchList(c)
					if err != nil {
						return nil, err
					}
					cert, err := certs.Find(list, args.Str("semester"), args.Str("lang"))
					if err != nil {
						return nil, err
					}
					u, name = cert.DownloadURL, strings.ReplaceAll(cert.Name, " ", "_")+".pdf"
				}
				data, _, err := certs.Download(c, u)
				if err != nil {
					return nil, err
				}
				if name == "" {
					name = "Studienbescheinigung.pdf"
				}
				return saveFile(a, data, name, args.Str("output_path"))
			}},
		{Name: "cis_page", Kind: Read, Desc: "Beliebige CIS-Infoseite als Text plus Downloads und Unterseiten lesen (z.B. /studium/pruefungen/pruefungsplan, /downloadcenter/studiengangsinformationen/modulhandbuecher).",
			Params: []Param{{Name: "path", Required: true, Desc: "Pfad oder CIS-URL (nur Seiten ohne Aktion)"}, {Name: "max_chars", Type: "integer", Desc: "Textlänge (Standard 20000)"}},
			Run: func(a *app.App, args Args) (any, error) {
				c, err := cis(a)
				if err != nil {
					return nil, err
				}
				n, _ := args.Int("max_chars", 20000)
				return pages.Fetch(c, args.Str("path"), int(n))
			}},
		{Name: "cis_download", Kind: Local, Desc: "Beliebige CIS-Datei herunterladen (dumpFile-/fileadmin-Links aus cis_page, Dokumente).",
			Params: []Param{{Name: "url", Required: true, Desc: "Datei-URL"}, {Name: "output_path", Desc: "Zielpfad/-ordner"}},
			Run: func(a *app.App, args Args) (any, error) {
				return downloadCIS(a, args.Str("url"), args.Str("output_path"))
			}},
	}
}

var downloadAction = map[string]bool{"document": true, "transcript": true}

func downloadCIS(a *app.App, rawURL, out string) (any, error) {
	c, err := cis(a)
	if err != nil {
		return nil, err
	}
	u := strings.ReplaceAll(rawURL, "&amp;", "&")
	act := htmlx.Action(u)
	if !(strings.Contains(u, "eID=dumpFile") || strings.Contains(u, "/fileadmin/") || downloadAction[act]) {
		return nil, fmt.Errorf("only file links (eID=dumpFile, /fileadmin/, document) can be downloaded")
	}
	data, ct, name, err := c.Download(u)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = fileNameFromURL(u)
		if name == "index.php" || name == "download" {
			name = "cis-download"
			switch {
			case strings.Contains(ct, "pdf"):
				name += ".pdf"
			case strings.Contains(ct, "calendar"):
				name += ".ics"
			}
		}
	}
	return saveFile(a, data, name, out)
}

func resolveExam(a *app.App, args Args) (*client.Client, *exams.Exam, error) {
	c, err := cis(a)
	if err != nil {
		return nil, nil, err
	}
	want := args.Str("action")
	if want != "register" && want != "deregister" {
		return nil, nil, fmt.Errorf("action must be 'register' or 'deregister'")
	}
	e, err := exams.Resolve(c, args.Str("exam_id"))
	if err != nil {
		return nil, nil, err
	}
	if e.Action != want {
		return nil, nil, fmt.Errorf("exam %s currently offers %q, not %q — refusing", e.ExamID, e.Action, want)
	}
	return c, e, nil
}

func prepareTransfer(a *app.App, args Args) (*client.Client, *forms.Submission, error) {
	c, err := cis(a)
	if err != nil {
		return nil, nil, err
	}
	path := a.ExpandPath(args.Str("file_path"))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read Auftragsklärung: %w", err)
	}
	lang := args.Str("language")
	if lang == "" {
		lang = "de"
	}
	s, err := transfer.PrepareRegister(c, transfer.RegisterRequest{No: args.Str("no"), Topic: args.Str("topic"), Module: args.Str("module"),
		Language: lang, Locked: args.BoolPtr("locked"), FileName: path, File: data})
	return c, s, err
}

func contactUpdate(a *app.App, args Args, send bool) (any, error) {
	c, err := cis(a)
	if err != nil {
		return nil, err
	}
	var subs []*forms.Submission
	var what []string
	if args.Has("email") || args.BoolPtr("infopost") != nil {
		s, err := profile.PrepareEmail(c, args.StrPtr("email"), args.BoolPtr("infopost"))
		if err != nil {
			return nil, err
		}
		subs, what = append(subs, s), append(what, "E-Mail/Infopost ändern")
	}
	if args.StrPtr("phone") != nil || args.StrPtr("mobile") != nil || args.StrPtr("fax") != nil {
		s, err := profile.PreparePhone(c, args.StrPtr("phone"), args.StrPtr("mobile"), args.StrPtr("fax"))
		if err != nil {
			return nil, err
		}
		subs, what = append(subs, s), append(what, "Telefonnummern ändern")
	}
	if len(subs) == 0 {
		return nil, fmt.Errorf("nothing to change: give email, infopost, phone, mobile or fax")
	}
	var out []any
	for i, s := range subs {
		if !send {
			out = append(out, submissionPreview(s, what[i]))
			continue
		}
		r, err := sendSubmission(c, s)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func addressUpdate(a *app.App, args Args, send bool) (any, error) {
	c, err := cis(a)
	if err != nil {
		return nil, err
	}
	s, err := profile.PrepareAddress(c, args.Str("type"), args.StrPtr("zusatz"), args.StrPtr("strasse"), args.StrPtr("plz"), args.StrPtr("ort"), args.Bool("delete", false))
	if err != nil {
		return nil, err
	}
	if !send {
		return submissionPreview(s, "Adresse ändern"), nil
	}
	return sendSubmission(c, s)
}

func sharingUpdate(a *app.App, args Args, send bool) (any, error) {
	c, err := cis(a)
	if err != nil {
		return nil, err
	}
	var subs []*forms.Submission
	var what []string
	if args.BoolPtr("noten_betrieb") != nil || args.BoolPtr("anmeldungen_betrieb") != nil {
		s, err := profile.PrepareFreigabe(c, args.BoolPtr("noten_betrieb"), args.BoolPtr("anmeldungen_betrieb"))
		if err != nil {
			return nil, err
		}
		subs, what = append(subs, s), append(what, "Freigabe für den Betrieb ändern")
	}
	km, err := args.Map("kommilitonen")
	if err != nil {
		return nil, err
	}
	if len(km) > 0 {
		levels := map[string]string{}
		for k, v := range km {
			levels[k] = strings.TrimSuffix(fmt.Sprint(v), ".0")
		}
		s, err := profile.PrepareDatenfreigabe(c, levels)
		if err != nil {
			return nil, err
		}
		subs, what = append(subs, s), append(what, "Datenfreigabe für Kommiliton:innen ändern")
	}
	if len(subs) == 0 {
		return nil, fmt.Errorf("nothing to change: give noten_betrieb, anmeldungen_betrieb or kommilitonen")
	}
	var out []any
	for i, s := range subs {
		if !send {
			out = append(out, submissionPreview(s, what[i]))
			continue
		}
		r, err := sendSubmission(c, s)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// ── timetable ───────────────────────────────────────────────────────────────

type agendaEvent struct {
	Date     string `json:"date"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Kind     string `json:"kind,omitempty"`
	ModuleNr string `json:"module_nr,omitempty"`
	Title    string `json:"title"`
	Lecturer string `json:"lecturer,omitempty"`
	Room     string `json:"room,omitempty"`
	Note     string `json:"note,omitempty"`
	Source   string `json:"source"`
	sortKey  time.Time
}

var weekdays = []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

func dayLabel(t time.Time) string { return weekdays[t.Weekday()] + " " + t.Format("02.01.2006") }

func parseRange(a *app.App, args Args) (time.Time, time.Time, error) {
	now := a.Now().In(a.Zone)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, a.Zone)
	if s := args.Str("from"); s != "" {
		t, err := time.ParseInLocation("2006-01-02", s, a.Zone)
		if err != nil {
			return from, from, fmt.Errorf("from must be YYYY-MM-DD")
		}
		from = t
	}
	days, err := args.Int("days", 7)
	if err != nil {
		return from, from, err
	}
	to := from.AddDate(0, 0, int(days))
	if s := args.Str("to"); s != "" {
		t, err := time.ParseInLocation("2006-01-02", s, a.Zone)
		if err != nil {
			return from, from, fmt.Errorf("to must be YYYY-MM-DD")
		}
		to = t.AddDate(0, 0, 1)
	}
	if !to.After(from) {
		return from, to, fmt.Errorf("empty date range")
	}
	return from, to, nil
}

func lectures(a *app.App, c *client.Client, args Args, from, to time.Time) ([]agendaEvent, []string, error) {
	zen := args.Str("zenturie")
	if zen == "" {
		p, err := profile.FetchPersonal(c)
		if err != nil {
			return nil, nil, err
		}
		zen = p.Zenturie
	}
	evs, files, err := timetable.Fetch(c, zen)
	if err != nil {
		return nil, nil, err
	}
	q := timetable.Query{From: from, To: to, Kind: args.Str("kind"), ModuleNr: args.Str("module_nr"), Text: args.Str("text")}
	var notes []string
	if !args.Bool("all_wpf", false) {
		if l, err := wahlpflicht.Fetch(c); err == nil && len(l.Chosen) > 0 {
			for _, m := range l.Chosen {
				q.ExcludeWPExcept = append(q.ExcludeWPExcept, m.Name)
			}
		} else {
			notes = append(notes, "eigene WPF unbekannt — alle WPF-Termine der Zenturie gezeigt")
		}
	}
	out := []agendaEvent{}
	if len(evs) > 0 {
		first, last := evs[0].Start, evs[0].End
		for _, e := range evs {
			if e.Start.Before(first) {
				first = e.Start
			}
			if e.End.After(last) {
				last = e.End
			}
		}
		if last.Before(from) || !first.Before(to) {
			notes = append(notes, fmt.Sprintf("Der veröffentlichte Stundenplan deckt nur %s – %s ab; für den angefragten Zeitraum gibt es (noch) keinen Plan.",
				first.In(a.Zone).Format("02.01.2006"), last.In(a.Zone).Format("02.01.2006")))
		}
	}
	for _, e := range timetable.Filter(evs, q) {
		s := e.Start.In(a.Zone)
		out = append(out, agendaEvent{Date: dayLabel(s), Start: s.Format("15:04"), End: e.End.In(a.Zone).Format("15:04"), Kind: e.Kind,
			ModuleNr: e.ModuleNr, Title: e.Title, Lecturer: e.Lecturer, Room: e.Room, Note: strings.TrimSpace(e.Pause + " " + e.Note),
			Source: "stundenplan", sortKey: s})
	}
	return out, append(notes, "Quellen: "+strings.Join(files, ", ")), nil
}

func timetableTool(a *app.App, c *client.Client, args Args) (any, error) {
	from, to, err := parseRange(a, args)
	if err != nil {
		return nil, err
	}
	evs, notes, err := lectures(a, c, args, from, to)
	if err != nil {
		return nil, err
	}
	return map[string]any{"from": from.Format("2006-01-02"), "to": to.AddDate(0, 0, -1).Format("2006-01-02"),
		"events": evs, "count": len(evs), "notes": notes}, nil
}

// ── progress ────────────────────────────────────────────────────────────────

var tmNr = regexp.MustCompile(`^TM(\d+)$`)

func progress(c *client.Client) (any, error) {
	sp, err := planning.FetchStudienplan(c)
	if err != nil {
		return nil, err
	}
	o, err := grades.Fetch(c)
	if err != nil {
		return nil, err
	}
	type modRow struct {
		ModuleNr string `json:"module_nr"`
		Title    string `json:"title"`
		Status   string `json:"status"`
		Grade    string `json:"grade,omitempty"`
		Credits  int    `json:"credits"`
		Exam     string `json:"exam,omitempty"`
		Attempt  int    `json:"attempt,omitempty"`
	}
	type sem struct {
		Semester int      `json:"semester"`
		Passed   int      `json:"passed"`
		Open     int      `json:"open"`
		Failed   int      `json:"failed"`
		Modules  []modRow `json:"modules"`
	}
	bySem := map[int]*sem{}
	planned, earned := 0, 0
	var prereqMissing []string
	for _, m := range sp.Modules {
		g := o.Find(m.ModuleNr)
		row := modRow{ModuleNr: m.ModuleNr, Title: m.Title, Credits: m.Credits, Status: grades.StatusOpen}
		if m.ExamForm != "" {
			row.Exam = m.ExamFormName
		}
		if g != nil {
			row.Status, row.Grade, row.Attempt = g.Status, g.Grade, g.Attempt
		}
		planned += m.Credits
		if row.Status == grades.StatusPassed {
			earned += m.Credits
		}
		s := m.ExamSemester
		if s == 0 && len(m.Semesters) > 0 {
			s = m.Semesters[len(m.Semesters)-1].Semester
		}
		if bySem[s] == nil {
			bySem[s] = &sem{Semester: s}
		}
		b := bySem[s]
		b.Modules = append(b.Modules, row)
		switch row.Status {
		case grades.StatusPassed:
			b.Passed++
		case grades.StatusFailed:
			b.Failed++
		default:
			b.Open++
		}
		if s > 0 && s <= 4 && row.Status != grades.StatusPassed {
			prereqMissing = append(prereqMissing, fmt.Sprintf("%s %s (%s)", m.ModuleNr, m.Title, row.Status))
		}
	}
	var sems []*sem
	for _, s := range bySem {
		sems = append(sems, s)
	}
	sort.Slice(sems, func(i, j int) bool { return sems[i].Semester < sems[j].Semester })

	tlPassed := map[int]bool{}
	for _, m := range o.Modules {
		if mm := tmNr.FindStringSubmatch(m.ModuleNr); mm != nil && m.Status == grades.StatusPassed {
			n, _ := strconv.Atoi(mm[1])
			tlPassed[n] = true
		}
	}
	for _, t := range o.Transfers {
		if strings.HasPrefix(strings.ToLower(t.Grade), "best") {
			n, _ := strconv.Atoi(t.No)
			tlPassed[n] = true
		}
	}
	var tlMissing []string
	for i := 1; i <= 5; i++ {
		if !tlPassed[i] {
			tlMissing = append(tlMissing, fmt.Sprintf("Transferleistung %d", i))
		}
	}
	tlCount := 0
	for i := 1; i <= 6; i++ {
		if tlPassed[i] {
			tlCount++
		}
	}
	st := o.Stats()
	return map[string]any{
		"credits": map[string]any{"module_earned": earned, "module_planned": planned, "transfer_earned": tlCount * 5, "transfer_planned": 30,
			"total_earned": earned + tlCount*5, "total_required": 210, "cis_credits_total": o.CreditsTotal},
		"average":     map[string]any{"cis": o.Average, "credit_weighted_recomputed": st.WeightedAverage},
		"semesters":   sems,
		"open_failed": st.Failed,
		"thesis_requirements": map[string]any{
			"fulfilled":                  len(prereqMissing) == 0 && len(tlMissing) == 0,
			"modules_until_4th_missing":  prereqMissing,
			"transferleistungen_missing": tlMissing,
			"rule":                       "Alle Modulprüfungen bis einschließlich 4. Semester bestanden und Transferleistungen 1–5 erfolgreich (Bachelorthesis-Seite im CIS).",
		},
	}, nil
}
