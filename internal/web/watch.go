package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Raindancer118/nak-api/internal/tools"
)

// Notification is one thing worth telling the owner about.
type Notification struct {
	ID    string    `json:"id"`
	At    time.Time `json:"at"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
	Body  string    `json:"body,omitempty"`
	URL   string    `json:"url,omitempty"`
	Read  bool      `json:"read"`
}

// generic titles go to ntfy unless details are switched on — ntfy.sh is a
// public server and a grade does not belong there.
var genericTitle = map[string]string{
	"grade": "Neue Note in naknak", "message": "Neue Nachricht in naknak", "news": "Neues in Moodle",
	"moodle": "Neue Moodle-Benachrichtigung", "deadline": "Frist in naknak",
}

const maxEvents = 200

type check struct {
	name  string
	tool  string
	args  tools.Args
	every time.Duration
	diff  func(raw json.RawMessage, seen map[string]string) []Notification
}

// The intervals are the whole point: they decide how many CIS/Moodle calls
// the watcher costs. A result the UI fetched recently is reused (see
// toolAtMost), and whatever the watcher fetches warms the UI's cache.
var checks = []check{
	{"grades", "cis_grades", tools.Args{}, 3 * time.Hour, diffGrades},
	{"news", "moodle_whats_new", tools.Args{"days": 2}, time.Hour, diffNews},
	{"messages", "moodle_conversations", tools.Args{"limit": 20}, 15 * time.Minute, diffConversations},
	{"moodle", "moodle_notifications", tools.Args{"limit": 20, "unread_only": true}, 15 * time.Minute, diffNotifications},
	{"deadlines", "nak_deadlines", tools.Args{"days": 3}, time.Hour, diffDeadlines},
}

type watchState struct {
	Seen      map[string]string    `json:"seen"`
	Last      map[string]time.Time `json:"last"`
	Baselined map[string]bool      `json:"baselined"`
	Events    []Notification       `json:"events"`
}

type watcher struct {
	s    *Server
	file string

	mu    sync.Mutex
	state watchState
	subs  map[chan Notification]struct{}
	run   sync.Mutex // one pass at a time
}

func newWatcher(s *Server) *watcher {
	w := &watcher{s: s, subs: map[chan Notification]struct{}{}}
	if s.app.ConfigDir != "" {
		w.file = filepath.Join(s.app.ConfigDir, "watch.json")
		if b, err := os.ReadFile(w.file); err == nil {
			json.Unmarshal(b, &w.state)
		}
	}
	if w.state.Seen == nil {
		w.state.Seen = map[string]string{}
	}
	if w.state.Last == nil {
		w.state.Last = map[string]time.Time{}
	}
	if w.state.Baselined == nil {
		w.state.Baselined = map[string]bool{}
	}
	return w
}

// Start runs due checks once a minute until ctx ends.
func (s *Server) StartWatcher(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		time.Sleep(20 * time.Second) // let the server come up first
		for {
			s.watch.runDue()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (w *watcher) runDue() {
	w.run.Lock()
	defer w.run.Unlock()
	cfg := w.s.app.Settings().Notify
	if cfg.Off {
		return
	}
	now := w.s.cfg.Now()
	if hr := now.In(w.s.app.Zone).Hour(); !cfg.Night && (hr < 7 || hr >= 23) {
		return
	}
	var fresh []Notification
	for _, c := range checks {
		w.mu.Lock()
		due := now.Sub(w.state.Last[c.name]) >= c.every
		w.mu.Unlock()
		if !due || w.s.reg.Get(c.tool) == nil {
			continue
		}
		raw, err := w.s.toolAtMost(c.tool, c.args, c.every)
		w.mu.Lock()
		w.state.Last[c.name] = now
		if err == nil {
			found := c.diff(raw, w.state.Seen)
			if w.state.Baselined[c.name] {
				fresh = append(fresh, found...)
			}
			// the first pass only learns what already exists
			w.state.Baselined[c.name] = true
		}
		w.mu.Unlock()
	}
	for _, n := range fresh {
		w.add(n)
	}
	w.save()
}

func (w *watcher) add(n Notification) {
	if n.ID == "" {
		b := make([]byte, 8)
		rand.Read(b)
		n.ID = hex.EncodeToString(b)
	}
	if n.At.IsZero() {
		n.At = w.s.cfg.Now()
	}
	w.mu.Lock()
	w.state.Events = append([]Notification{n}, w.state.Events...)
	if len(w.state.Events) > maxEvents {
		w.state.Events = w.state.Events[:maxEvents]
	}
	for ch := range w.subs {
		select {
		case ch <- n:
		default: // a slow tab misses a live event, not the list
		}
	}
	w.mu.Unlock()
	w.push(n)
}

func (w *watcher) push(n Notification) {
	cfg := w.s.app.Settings().Notify
	if cfg.NtfyURL == "" {
		return
	}
	title, body := n.Title, n.Body
	if !cfg.NtfyDetails {
		title, body = genericTitle[n.Kind], "Öffne naknak für Details."
		if title == "" {
			title = "Neues in naknak"
		}
	}
	req, err := http.NewRequest(http.MethodPost, cfg.NtfyURL, strings.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Title", title)
	req.Header.Set("Tags", "duck")
	c := &http.Client{Timeout: 10 * time.Second}
	if res, err := c.Do(req); err != nil {
		log.Printf("ntfy: %v", err)
	} else {
		res.Body.Close()
	}
}

func (w *watcher) list() []Notification {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append(make([]Notification, 0, len(w.state.Events)), w.state.Events...)
}

func (w *watcher) markRead() {
	w.mu.Lock()
	for i := range w.state.Events {
		w.state.Events[i].Read = true
	}
	w.mu.Unlock()
	w.save()
}

func (w *watcher) save() {
	if w.file == "" {
		return
	}
	w.mu.Lock()
	b, err := json.Marshal(w.state)
	w.mu.Unlock()
	if err != nil {
		return
	}
	if os.WriteFile(w.file+".tmp", b, 0o600) == nil {
		os.Rename(w.file+".tmp", w.file)
	}
}

func (w *watcher) subscribe() chan Notification {
	ch := make(chan Notification, 8)
	w.mu.Lock()
	w.subs[ch] = struct{}{}
	w.mu.Unlock()
	return ch
}

func (w *watcher) unsubscribe(ch chan Notification) {
	w.mu.Lock()
	delete(w.subs, ch)
	w.mu.Unlock()
}

// toolAtMost returns a cached result younger than maxAge, otherwise fetches.
func (s *Server) toolAtMost(name string, args tools.Args, maxAge time.Duration) (json.RawMessage, error) {
	t := s.reg.Get(name)
	k, _ := json.Marshal(args)
	key := t.Name + " " + string(k)
	if e, ok := s.store.get(key); ok && usable(e) && s.cfg.Now().Sub(e.At) < maxAge {
		return e.Res, nil
	}
	res, _, err := s.refresh(t, key, args)
	return res, err
}

// ── diffs ───────────────────────────────────────────────────────────────────

// seenChange records key=value and reports whether value is new.
func seenChange(seen map[string]string, key, value string) bool {
	old, ok := seen[key]
	seen[key] = value
	return !ok || old != value
}

func list(raw json.RawMessage, field string) []map[string]any {
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil && obj[field] != nil {
		json.Unmarshal(obj[field], &arr)
	}
	return arr
}

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func diffGrades(raw json.RawMessage, seen map[string]string) []Notification {
	var g struct {
		Overview struct {
			Modules []map[string]any `json:"modules"`
		} `json:"overview"`
	}
	json.Unmarshal(raw, &g)
	var out []Notification
	for _, m := range g.Overview.Modules {
		nr, grade := str(m, "module_nr"), str(m, "grade")
		if nr == "" || grade == "" {
			continue
		}
		old, had := seen["grade:"+nr]
		if seenChange(seen, "grade:"+nr, grade) {
			title := "Neue Note: " + str(m, "title") + " " + grade
			if had && old != "" {
				title = "Note geändert: " + str(m, "title") + " " + grade
			}
			out = append(out, Notification{Kind: "grade", Title: title, Body: str(m, "status"), URL: "#/modul/" + strings.SplitN(nr, ",", 2)[0]})
		}
	}
	return out
}

func diffNews(raw json.RawMessage, seen map[string]string) []Notification {
	var out []Notification
	for _, x := range list(raw, "changes") {
		changes := ""
		if arr, ok := x["changes"].([]any); ok {
			parts := make([]string, 0, len(arr))
			for _, c := range arr {
				parts = append(parts, fmt.Sprint(c))
			}
			changes = strings.Join(parts, " · ")
		}
		key := "news:" + str(x, "courseid") + ":" + str(x, "cmid") + ":" + changes
		if seenChange(seen, key, "1") {
			out = append(out, Notification{Kind: "news", Title: "Neu in " + str(x, "course") + ": " + str(x, "activity"), Body: changes, URL: "#/kurs/" + str(x, "courseid")})
		}
	}
	return out
}

func diffConversations(raw json.RawMessage, seen map[string]string) []Notification {
	var out []Notification
	for _, c := range list(raw, "conversations") {
		id := str(c, "conversationid")
		if id == "" {
			continue
		}
		if seenChange(seen, "conv:"+id, str(c, "last_time")) {
			out = append(out, Notification{Kind: "message", Title: "Neue Nachricht: " + str(c, "name"), Body: str(c, "last_message"), URL: "#/nachrichten/" + id})
		}
	}
	return out
}

func diffNotifications(raw json.RawMessage, seen map[string]string) []Notification {
	var out []Notification
	for _, n := range list(raw, "notifications") {
		id := str(n, "id")
		if id != "" && seenChange(seen, "notif:"+id, "1") {
			out = append(out, Notification{Kind: "moodle", Title: str(n, "subject"), URL: "#/neu"})
		}
	}
	return out
}

func diffDeadlines(raw json.RawMessage, seen map[string]string) []Notification {
	var out []Notification
	for _, d := range list(raw, "deadlines") {
		days := str(d, "in_days")
		if days != "0" && days != "1" {
			continue
		}
		if seenChange(seen, "due:"+str(d, "what")+"|"+str(d, "when"), "1") {
			title := "Frist morgen: "
			if days == "0" {
				title = "Frist heute: "
			}
			url := "#/abgaben"
			if !strings.HasPrefix(str(d, "source"), "moodle") {
				url = "#/woche"
			}
			out = append(out, Notification{Kind: "deadline", Title: title + str(d, "what"), Body: str(d, "when"), URL: url})
		}
	}
	return out
}

// ── HTTP ────────────────────────────────────────────────────────────────────

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	evs := s.watch.list()
	unread := 0
	for _, e := range evs {
		if !e.Read {
			unread++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": evs, "unread": unread, "enabled": !s.app.Settings().Notify.Off})
}

func (s *Server) notificationsRead(w http.ResponseWriter, r *http.Request) {
	s.watch.markRead()
	writeJSON(w, http.StatusOK, map[string]any{"read": true})
}

// events streams notifications live (Server-Sent Events) to open tabs.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{}) // long-lived, unlike every other response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // nginx/NPM: don't buffer the stream
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": naknak\n\n")
	rc.Flush()
	ch := s.watch.subscribe()
	defer s.watch.unsubscribe(ch)
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case n := <-ch:
			b, _ := json.Marshal(n)
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", b)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}
