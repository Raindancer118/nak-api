package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/eduvault"
	"github.com/Raindancer118/nak-api/internal/grades"
	"github.com/Raindancer118/nak-api/internal/moodle"
	"github.com/Raindancer118/nak-api/internal/planning"
)

// moduleTitle resolves a module number to its CIS title (grades first, then
// the study plan, which also lists modules without a result yet).
func moduleTitle(a *app.App, nr string) (string, error) {
	c, err := a.CIS()
	if err != nil {
		return "", err
	}
	if o, err := grades.Fetch(c); err == nil {
		if m := o.Find(nr); m != nil && m.Title != "" {
			return m.Title, nil
		}
	}
	if sp, err := planning.FetchStudienplan(c); err == nil {
		for _, m := range sp.Modules {
			if m.ModuleNr == nr {
				return m.Title, nil
			}
		}
	}
	return "", fmt.Errorf("Modul %s nicht im CIS gefunden", nr)
}

func eduvaultTools() []*Tool {
	return []*Tool{
		{Name: "eduvault_search", Kind: Read, Desc: "Sucht in EduVault (eduvault4.de) nach Altklausuren, Probeklausuren, Lernzetteln und Skripten. Braucht in nak hinterlegte EduVault-Zugangsdaten.",
			Params: []Param{{Name: "q", Desc: "Freitext"}, {Name: "modul", Desc: "Modulname, z.B. Datenbanksysteme"}, {Name: "dozent", Desc: "Dozent"},
				{Name: "jahr", Desc: "Jahr"}, {Name: "type", Type: "array:string", Desc: "exam, probeklausur, note, script (Standard alle)"}, {Name: "limit", Type: "integer", Desc: "max. Treffer (≤100)"}},
			Run: func(a *app.App, args Args) (any, error) {
				ev, err := a.EduVault()
				if err != nil {
					return nil, err
				}
				limit, err := args.Int("limit", 50)
				if err != nil {
					return nil, err
				}
				return ev.Search(eduvault.Query{Q: args.Str("q"), Modul: args.Str("modul"), Dozent: args.Str("dozent"), Jahr: args.Str("jahr"), Types: args.Strs("type"), Limit: int(limit)})
			}},
		{Name: "eduvault_module_exams", Kind: Read, Desc: "Altklausuren und Probeklausuren zu einem Modul aus EduVault, über alle Schreibweisen (\"Mathematik 2\" / \"Mathematik II\"), neueste zuerst.",
			Params: []Param{{Name: "module_nr", Desc: "Modulnummer, z.B. I168 (Titel kommt aus dem CIS)"}, {Name: "title", Desc: "Modultitel, statt module_nr"},
				{Name: "notes", Type: "boolean", Desc: "auch Lernzettel und Skripte"}},
			Run: func(a *app.App, args Args) (any, error) {
				ev, err := a.EduVault()
				if err != nil {
					return nil, err
				}
				title := args.Str("title")
				if title == "" {
					nr := strings.ToUpper(args.Str("module_nr"))
					if nr == "" {
						return nil, fmt.Errorf("module_nr oder title angeben")
					}
					if title, err = moduleTitle(a, nr); err != nil {
						return nil, err
					}
				}
				types := []string{"exam", "probeklausur"}
				if args.Bool("notes", false) {
					types = append(types, "note", "script")
				}
				list, err := ev.ForModule(title, types...)
				if err != nil {
					return nil, err
				}
				return map[string]any{"module": title, "spellings": eduvault.Spellings(title), "count": len(list), "documents": list}, nil
			}},
		{Name: "eduvault_download", Kind: Local, Desc: "Lädt ein EduVault-Dokument (PDF) herunter und gibt den lokalen Pfad zurück.",
			Params: []Param{{Name: "exam_id", Required: true, Desc: "id aus eduvault_search/eduvault_module_exams"}, {Name: "out", Desc: "Zielpfad/-ordner (Standard Download-Ordner/eduvault)"}},
			Run: func(a *app.App, args Args) (any, error) {
				ev, err := a.EduVault()
				if err != nil {
					return nil, err
				}
				data, _, name, err := ev.File(args.Str("exam_id"))
				if err != nil {
					return nil, err
				}
				name = filepath.Base(strings.TrimSpace(name))
				if name == "" || name == "." || name == "/" {
					name = args.Str("exam_id") + ".pdf"
				}
				out := args.Str("out")
				if out == "" {
					dir := filepath.Join(a.DownloadDir, "eduvault")
					if err := os.MkdirAll(dir, 0o755); err != nil {
						return nil, err
					}
					out = moodle.FreeName(dir, name)
				}
				return saveFile(a, data, name, out)
			}},
	}
}
