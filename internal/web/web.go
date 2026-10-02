// Package web is the browser front end of nak: a single-user HTTP server
// that exposes the tool registry as a JSON API next to the embedded UI.
// One owner, one token — there are no accounts.
package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/drift"
	"github.com/Raindancer118/nak-api/internal/tools"
)

//go:embed ui
var uiFS embed.FS

const (
	cookieName       = "nak_session"
	confirmHeader    = "X-Nak-Confirm"
	tokenFile        = "web-token"
	maxLoginFailures = 10
	loginWindow      = 15 * time.Minute
	maxBody          = 1 << 20
)

type Config struct {
	Token    string
	CacheTTL time.Duration
	Version  string
	Now      func() time.Time
}

type Server struct {
	app *app.App
	reg *tools.Registry
	cfg Config
	ui  fs.FS

	// The CIS client keeps one cookie session and relogs in on expiry;
	// concurrent tool calls would race on that, so calls run one at a time.
	callMu sync.Mutex

	cacheMu sync.Mutex
	cache   map[string]cached

	loginMu sync.Mutex
	logins  map[string]*attempts
	login   *template.Template
}

type cached struct {
	res any
	at  time.Time
}

type attempts struct {
	fails int
	first time.Time
}

func New(a *app.App, reg *tools.Registry, cfg Config) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	ui, err := fs.Sub(uiFS, "ui")
	if err != nil {
		panic(err)
	}
	return &Server{
		app: a, reg: reg, cfg: cfg, ui: ui,
		cache:  map[string]cached{},
		logins: map[string]*attempts{},
		login:  template.Must(template.ParseFS(ui, "login.html")),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("POST /logout", s.logout)
	mux.Handle("GET /api/tools", s.authed(http.HandlerFunc(s.listTools)))
	mux.Handle("POST /api/tools/{name}", s.authed(http.HandlerFunc(s.callTool)))
	mux.Handle("GET /files/{path...}", s.authed(http.HandlerFunc(s.file)))
	mux.Handle("GET /assets/", http.FileServerFS(s.ui))
	mux.HandleFunc("GET /{$}", s.index)
	return s.headers(mux)
}

// ── middleware ─────────────────────────────────────────────────────────────

func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if secure(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sessionValue() string {
	m := hmac.New(sha256.New, []byte(s.cfg.Token))
	m.Write([]byte("nak-web-session-v1"))
	return hex.EncodeToString(m.Sum(nil))
}

func eq(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func (s *Server) isAuthed(r *http.Request) bool {
	if b, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return eq(strings.TrimSpace(b), s.cfg.Token)
	}
	c, err := r.Cookie(cookieName)
	return err == nil && eq(c.Value, s.sessionValue())
}

func (s *Server) authed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthed(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "nicht angemeldet"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin rejects cross-site requests. The session cookie is SameSite=Strict
// already; this also covers browsers that send Origin but no Sec-Fetch-Site.
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// ── pages ──────────────────────────────────────────────────────────────────

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.cfg.Version})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthed(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	b, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		http.Error(w, "UI missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) locked(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	a := s.logins[ip]
	if a == nil {
		return false
	}
	if s.cfg.Now().Sub(a.first) > loginWindow {
		delete(s.logins, ip)
		return false
	}
	return a.fails >= maxLoginFailures
}

func (s *Server) failed(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	a := s.logins[ip]
	if a == nil {
		a = &attempts{first: s.cfg.Now()}
		s.logins[ip] = a
	}
	a.fails++
}

// tryLogin answers the login attempt completely (cookie + redirect or error page).
func (s *Server) tryLogin(w http.ResponseWriter, r *http.Request, tok string) {
	ip := clientIP(r)
	if s.locked(ip) {
		s.renderLogin(w, http.StatusTooManyRequests, "Zu viele Fehlversuche. Bitte in ein paar Minuten erneut versuchen.")
		return
	}
	if !eq(strings.TrimSpace(tok), s.cfg.Token) {
		s.failed(ip)
		s.renderLogin(w, http.StatusUnauthorized, "Der Zugangsschlüssel stimmt nicht.")
		return
	}
	s.loginMu.Lock()
	delete(s.logins, ip)
	s.loginMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: s.sessionValue(), Path: "/",
		MaxAge: 30 * 24 * 3600, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// loginPage never accepts a token from the query string (it would end up in
// browser history and proxy logs); the log link carries it as #fragment and
// login.js posts it.
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.renderLogin(w, http.StatusOK, "")
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-site request", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	s.tryLogin(w, r, r.PostFormValue("token"))
}

func (s *Server) renderLogin(w http.ResponseWriter, status int, msg string) {
	var buf bytes.Buffer
	if err := s.login.Execute(&buf, map[string]string{"Error": msg, "Version": s.cfg.Version}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-site request", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ── API ────────────────────────────────────────────────────────────────────

var kindNames = map[tools.Kind]string{tools.Read: "read", tools.Local: "local", tools.Write: "write"}

func (s *Server) listTools(w http.ResponseWriter, r *http.Request) {
	type param struct {
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		Desc     string   `json:"description"`
		Required bool     `json:"required,omitempty"`
		Enum     []string `json:"enum,omitempty"`
	}
	type tool struct {
		Name   string  `json:"name"`
		Kind   string  `json:"kind"`
		Desc   string  `json:"description"`
		Params []param `json:"params"`
	}
	out := []tool{}
	for _, t := range s.reg.All() {
		e := tool{Name: t.Name, Kind: kindNames[t.Kind], Desc: t.Desc, Params: []param{}}
		for _, p := range t.Params {
			typ := p.Type
			if typ == "" {
				typ = "string"
			}
			e.Params = append(e.Params, param{p.Name, typ, p.Desc, p.Required, p.Enum})
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": out, "read_only": s.app.ReadOnly, "version": s.cfg.Version})
}

func (s *Server) callTool(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "cross-site request"})
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"})
		return
	}
	t := s.reg.Get(r.PathValue("name"))
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown tool " + r.PathValue("name")})
		return
	}
	args := tools.Args{}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err == nil && len(bytes.TrimSpace(body)) > 0 {
		err = json.Unmarshal(body, &args)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body must be a JSON object: " + err.Error()})
		return
	}
	for _, p := range t.Params {
		if p.Required && !args.Has(p.Name) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": p.Name + " is required"})
			return
		}
	}
	if t.Kind == tools.Local {
		// Downloads stay in the download dir so /files can serve them.
		delete(args, "out")
	}
	if t.Kind == tools.Write && args.Bool("confirm", false) && r.Header.Get(confirmHeader) != "JA" {
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "verbindliche Aktion: confirm=true braucht zusätzlich den Header " + confirmHeader + ": JA"})
		return
	}

	key := ""
	if t.Kind == tools.Read && !t.MayWrite {
		k, _ := json.Marshal(args) // map keys are sorted
		key = t.Name + " " + string(k)
	}
	fresh := r.URL.Query().Get("fresh") == "1"
	if key != "" && !fresh {
		if c, ok := s.fromCache(key); ok {
			s.respond(w, t, c.res, c.at, true)
			return
		}
	}

	s.callMu.Lock()
	if key != "" && !fresh {
		// another request may have fetched it while this one waited
		if c, ok := s.fromCache(key); ok {
			s.callMu.Unlock()
			s.respond(w, t, c.res, c.at, true)
			return
		}
	}
	res, err := s.reg.Call(s.app, t.Name, args)
	at := s.cfg.Now()
	s.callMu.Unlock()

	if err != nil {
		out := map[string]any{"error": err.Error(), "tool": t.Name}
		var d *drift.Error
		if errors.As(err, &d) {
			out["drift"] = true
			out["report_url"] = tools.DriftReportURL(err, at)
		}
		writeJSON(w, http.StatusBadGateway, out)
		return
	}
	if key != "" {
		s.cacheMu.Lock()
		s.cache[key] = cached{res, at}
		s.cacheMu.Unlock()
	} else if t.Kind == tools.Write && args.Bool("confirm", false) {
		// anything read before a binding change may be stale now
		s.cacheMu.Lock()
		s.cache = map[string]cached{}
		s.cacheMu.Unlock()
	}
	s.respond(w, t, res, at, false)
}

func (s *Server) fromCache(key string) (cached, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	c, ok := s.cache[key]
	if !ok || s.cfg.Now().Sub(c.at) >= s.cfg.CacheTTL {
		return cached{}, false
	}
	return c, true
}

func (s *Server) respond(w http.ResponseWriter, t *tools.Tool, res any, at time.Time, hit bool) {
	out := map[string]any{"tool": t.Name, "result": res, "cached": hit, "fetched_at": at.Format(time.RFC3339)}
	if t.Kind == tools.Local {
		if m, ok := res.(map[string]any); ok {
			if p, ok := m["path"].(string); ok {
				if u := s.downloadURL(p); u != "" {
					out["download_url"] = u
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) downloadURL(p string) string {
	rel, err := filepath.Rel(s.app.DownloadDir, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, x := range parts {
		parts[i] = url.PathEscape(x)
	}
	return "/files/" + strings.Join(parts, "/")
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	// os.OpenInRoot refuses "..", absolute paths and symlinks leaving the dir.
	f, err := os.OpenInRoot(s.app.DownloadDir, r.PathValue("path"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": st.Name()}))
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

// LoadOrCreateToken returns the access token stored in dir, creating a
// random one (mode 0600) on first start.
func LoadOrCreateToken(dir string) (tok string, created bool, err error) {
	p := filepath.Join(dir, tokenFile)
	if b, err := os.ReadFile(p); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, false, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	tok = base64.RawURLEncoding.EncodeToString(buf)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(p, []byte(tok+"\n"), 0o600); err != nil {
		return "", false, err
	}
	return tok, true, nil
}
