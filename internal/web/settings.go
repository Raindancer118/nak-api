package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/eduvault"
)

func (s *Server) sameOriginOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "cross-site request"})
			return
		}
		next(w, r)
	}
}

// jsonOnly: same-origin and a JSON body — a cross-site form can send neither.
func (s *Server) jsonOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.sameOriginOnly(func(w http.ResponseWriter, r *http.Request) {
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"})
			return
		}
		next(w, r)
	})
}

// hint shows enough of a credential to recognise it, never the secret part.
func hint(tok string) string {
	if i := strings.LastIndex(tok, "_"); i > 0 {
		return tok[:i] + "_…"
	}
	if len(tok) > 6 {
		return tok[:6] + "…"
	}
	return "…"
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	ev := map[string]any{"configured": false, "source": s.app.EduVaultSource(), "url": eduvault.DefaultURL}
	if c, err := s.app.EduVault(); err == nil {
		ev["configured"], ev["url"], ev["token_hint"] = true, c.Base, hint(c.Token)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"eduvault":   ev,
		"notify":     s.app.Settings().Notify,
		"nav":        s.app.Settings().NavOrDefault(),
		"nav_items":  app.NavItems,
		"home":       s.app.Settings().HomeOrDefault(),
		"operator":   map[string]string{"name": s.cfg.Operator, "contact": s.cfg.OperatorContact},
		"home_items": app.HomeItems,
		"account":    s.accountInfo(),
		"read_only":  s.app.ReadOnly,
		"demo":       s.cfg.Demo,
		"version":    s.cfg.Version,
		"data_dir":   s.app.ConfigDir,
	})
}

func (s *Server) putEduVault(w http.ResponseWriter, r *http.Request) {
	var in app.EduVaultSettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body must be JSON"})
		return
	}
	in.URL, in.Token, in.Secret = strings.TrimSpace(in.URL), strings.TrimSpace(in.Token), strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in.Secret), " ", ""))
	if in.URL != "" && !strings.HasPrefix(in.URL, "https://") && !strings.HasPrefix(in.URL, "http://127.0.0.1") && !strings.HasPrefix(in.URL, "http://localhost") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "EduVault-Adresse muss mit https:// beginnen"})
		return
	}
	// check before saving: a typo should fail here, not on the module page
	if err := eduvault.New(in.URL, in.Token, in.Secret).Login(); err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, eduvault.ErrAuth) && !errors.Is(err, eduvault.ErrNotConfigured) {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	st := s.app.Settings()
	st.EduVault = in
	if err := s.app.SaveSettings(st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.store.clear()
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (s *Server) deleteEduVault(w http.ResponseWriter, r *http.Request) {
	st := s.app.Settings()
	st.EduVault = app.EduVaultSettings{}
	if err := s.app.SaveSettings(st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.store.clear()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) accountInfo() map[string]any {
	return map[string]any{"user": s.app.AccountUser(), "source": s.app.AccountSource()}
}

func (s *Server) putNotify(w http.ResponseWriter, r *http.Request) {
	var in app.NotifySettings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body must be JSON"})
		return
	}
	in.NtfyURL = strings.TrimSpace(in.NtfyURL)
	if in.NtfyURL != "" && !strings.HasPrefix(in.NtfyURL, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ntfy-Adresse muss mit https:// beginnen, z.B. https://ntfy.sh/mein-geheimes-thema"})
		return
	}
	st := s.app.Settings()
	st.Notify = in
	if err := s.app.SaveSettings(st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// putNav stores the navigation bar: 2 to 7 entries (a phone fits seven).
func (s *Server) putNav(w http.ResponseWriter, r *http.Request) {
	s.putList(w, r, "nav", app.NavItems, 2, 7, "Die Leiste braucht 2 bis 7 Einträge.", func(st *app.Settings, v []string) []string {
		st.Nav = v
		return st.NavOrDefault()
	})
}

// putHome stores the dashboard tiles: at least one.
func (s *Server) putHome(w http.ResponseWriter, r *http.Request) {
	s.putList(w, r, "home", app.HomeItems, 1, len(app.HomeItems), "Die Startseite braucht mindestens eine Kachel.", func(st *app.Settings, v []string) []string {
		st.Home = v
		return st.HomeOrDefault()
	})
}

// putList takes {"<key>": [ids]}: known, distinct, between min and max; an
// empty list goes back to the default.
func (s *Server) putList(w http.ResponseWriter, r *http.Request, key string, items []string, min, max int, sizeErr string, set func(*app.Settings, []string) []string) {
	var in map[string][]string
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body must be JSON"})
		return
	}
	list := in[key]
	if len(list) > 0 {
		known := map[string]bool{}
		for _, id := range items {
			known[id] = true
		}
		seen := map[string]bool{}
		for _, id := range list {
			if !known[id] || seen[id] {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unbekannter oder doppelter Eintrag: " + id})
				return
			}
			seen[id] = true
		}
		if len(list) < min || len(list) > max {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": sizeErr})
			return
		}
	}
	st := s.app.Settings()
	shown := set(&st, list)
	if err := s.app.SaveSettings(st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, key: shown})
}
