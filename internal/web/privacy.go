package web

import (
	"archive/zip"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const exportReadme = `naknak – Datenexport

Alles, was diese naknak-Instanz über dich gespeichert hat (Stand: siehe Dateidatum).

account.json        dein NAK-Konto (ohne Passwort)
settings.json       Einstellungen (EduVault-Zugang ohne Geheimnisse)
cache.json          zwischengespeicherte Antworten von CIS, Moodle und EduVault
history.json        gesehene Klausurtermine und Notenstände
notifications.json  Benachrichtigungen
stats.json          wie oft CIS/Moodle gefragt wurden
writes.log          jede verbindliche Aktion (Ziel und Feldnamen, nie Werte)

Passwort, Zugangsschlüssel, Kalender-Token und EduVault-Secret sind bewusst
nicht enthalten.
`

// export is the data access request: one zip with everything stored about
// the owner, secrets removed.
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="naknak-export-`+s.cfg.Now().Format("2006-01-02")+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	defer zw.Close()
	add := func(name string, v any) {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: s.cfg.Now()})
		if err != nil {
			return
		}
		if b, ok := v.([]byte); ok {
			f.Write(b)
			return
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		enc.Encode(v)
	}
	add("README.txt", []byte(exportReadme))
	add("account.json", map[string]any{"user": s.app.AccountUser(), "source": s.app.AccountSource()})
	st := s.app.Settings()
	ev := map[string]any{"url": st.EduVault.URL, "configured": st.EduVault.Token != ""}
	if st.EduVault.Token != "" {
		ev["token"] = hint(st.EduVault.Token)
	}
	add("settings.json", map[string]any{"eduvault": ev, "notify": st.Notify})
	s.store.mu.Lock()
	cache := make(map[string]entry, len(s.store.entries))
	for k, v := range s.store.entries {
		cache[k] = v
	}
	s.store.mu.Unlock()
	add("cache.json", cache)
	add("history.json", s.history.snapshot())
	add("notifications.json", s.watch.list())
	since, f, h, e, per := s.stats.snapshot()
	add("stats.json", map[string]any{"since": since, "fetches": f, "cache_hits": h, "errors": e, "tools": per})
	if b, err := os.ReadFile(filepath.Join(s.app.ConfigDir, "writes.log")); err == nil {
		add("writes.log", b)
	}
}

// reset deletes the account and every file naknak keeps, and rotates the
// access key so every open session ends. The instance can then be claimed
// again by the next NAK login.
func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&in)
	if in.Confirm != "LÖSCHEN" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": `zum Bestätigen "LÖSCHEN" senden`})
		return
	}
	dir := s.app.ConfigDir
	for _, f := range []string{"settings.json", "web-cache.json", "web-cache.json.tmp", "watch.json", "history.json", calendarFile, "writes.log", "audit.log"} {
		os.Remove(filepath.Join(dir, f))
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "session*.json")); m != nil {
		for _, f := range m {
			os.Remove(f)
		}
	}
	os.RemoveAll(filepath.Join(dir, "uploads"))
	os.RemoveAll(filepath.Join(dir, "downloads"))
	if dl := s.app.DownloadDir; dl != "" && strings.HasPrefix(filepath.Clean(dl), filepath.Clean(dir)) {
		os.RemoveAll(dl) // only when the downloads live inside the data dir
	}
	s.app.ClearAccount()
	s.store.clear()
	s.watch.reset()
	s.history.reset()
	s.stats.reset(s.cfg.Now())

	s.rotateKey()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"reset": true, "at": time.Now().Format(time.RFC3339)})
}

// rotateKey replaces the access key: every session cookie (they are derived
// from it) and every Bearer use of the old key stop working at once.
func (s *Server) rotateKey() {
	b := make([]byte, 32)
	rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	s.tokenMu.Lock()
	s.cfg.Token = tok
	s.tokenMu.Unlock()
	if os.Getenv("NAK_WEB_TOKEN") == "" {
		os.WriteFile(filepath.Join(s.app.ConfigDir, tokenFile), []byte(tok+"\n"), 0o600)
	}
}

func (s *Server) revokeSessions(w http.ResponseWriter, r *http.Request) {
	s.rotateKey()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}
