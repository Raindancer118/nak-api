package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/tools"
)

// fakeSources stands in for the five tools the watcher reads.
type fakeSources struct {
	mu        sync.Mutex
	grade     string
	news      []map[string]any
	convTime  string
	notifs    []map[string]any
	deadlines []map[string]any
	calls     map[string]int
}

func (f *fakeSources) registry() *tools.Registry {
	r := tools.NewRegistry()
	run := func(name string, out func() any) *tools.Tool {
		return &tools.Tool{Name: name, Kind: tools.Read, Desc: name, Params: []tools.Param{{Name: "days", Type: "integer"}, {Name: "limit", Type: "integer"}, {Name: "unread_only", Type: "boolean"}},
			Run: func(*app.App, tools.Args) (any, error) {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.calls[name]++
				return out(), nil
			}}
	}
	r.Add(
		run("cis_grades", func() any {
			return map[string]any{"overview": map[string]any{"modules": []map[string]any{
				{"module_nr": "I160", "title": "Datenbanksysteme", "grade": f.grade, "status": "bestanden"},
				{"module_nr": "I151", "title": "Softwaretechnik", "grade": "2,0", "status": "bestanden"},
			}}}
		}),
		run("moodle_whats_new", func() any { return f.news }),
		run("moodle_conversations", func() any {
			return []map[string]any{{"conversationid": 7, "name": "Prof. Muster", "last_message": "Hallo zusammen", "last_time": f.convTime}}
		}),
		run("moodle_notifications", func() any { return f.notifs }),
		run("nak_deadlines", func() any { return map[string]any{"deadlines": f.deadlines} }),
	)
	return r
}

type watchHarness struct {
	srv   *Server
	f     *fakeSources
	now   *time.Time
	ntfy  []string
	ntfyM sync.Mutex
}

func newWatchHarness(t *testing.T, notify app.NotifySettings) *watchHarness {
	t.Helper()
	f := &fakeSources{grade: "", convTime: "2026-10-02 08:00", calls: map[string]int{}}
	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	wh := &watchHarness{f: f, now: &now}
	push := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		wh.ntfyM.Lock()
		wh.ntfy = append(wh.ntfy, r.Header.Get("Title")+" | "+string(b))
		wh.ntfyM.Unlock()
	}))
	t.Cleanup(push.Close)
	a := app.FromEnv()
	a.ConfigDir = t.TempDir()
	a.Zone, _ = time.LoadLocation("Europe/Berlin")
	notify.NtfyURL = strings.Replace(notify.NtfyURL, "PUSH", push.URL, 1)
	a.SaveSettings(app.Settings{Notify: notify})
	wh.srv = New(a, f.registry(), Config{Token: token, Version: "t", Now: func() time.Time { return *wh.now }})
	return wh
}

func (w *watchHarness) events(t *testing.T) []Notification {
	t.Helper()
	return w.srv.watch.list()
}

func TestWatcherBaselineThenChanges(t *testing.T) {
	w := newWatchHarness(t, app.NotifySettings{NtfyURL: "PUSH/naknak"})
	w.srv.watch.runDue()
	if n := len(w.events(t)); n != 0 {
		t.Fatalf("first run must only take a baseline, got %d events", n)
	}
	// a grade appears, a message arrives, Moodle has news, a deadline gets urgent
	w.f.mu.Lock()
	w.f.grade = "1,7"
	w.f.convTime = "2026-10-02 11:00"
	w.f.news = []map[string]any{{"course": "DM2TI25", "courseid": 7566, "cmid": 1, "activity": "probeklausur", "type": "resource", "changes": []string{"1 neue/geänderte Datei(en)"}}}
	w.f.notifs = []map[string]any{{"id": 99, "subject": "Neue Bewertung in Übungsblatt 3", "time": "2026-10-02 11:00", "unread": true}}
	w.f.deadlines = []map[string]any{{"when": "Sa 03.10.2026 23:59", "in_days": 1, "what": "Abgabe Projekt", "kind": "moodle_assign", "source": "moodle", "urgent": true}}
	w.f.mu.Unlock()
	*w.now = w.now.Add(4 * time.Hour) // every check is due again
	w.srv.watch.runDue()
	got := w.events(t)
	titles := ""
	for _, e := range got {
		titles += e.Title + "\n"
	}
	for _, want := range []string{"Neue Note: Datenbanksysteme 1,7", "Neue Nachricht: Prof. Muster", "Neu in DM2TI25: probeklausur", "Neue Bewertung in Übungsblatt 3", "Frist morgen: Abgabe Projekt"} {
		if !strings.Contains(titles, want) {
			t.Errorf("missing %q in\n%s", want, titles)
		}
	}
	// ntfy without details by default: no grade on a public server
	w.ntfyM.Lock()
	pushed := strings.Join(w.ntfy, "\n")
	w.ntfyM.Unlock()
	if len(w.ntfy) != len(got) || strings.Contains(pushed, "1,7") || !strings.Contains(pushed, "Neue Note") {
		t.Fatalf("ntfy pushes (%d for %d events):\n%s", len(w.ntfy), len(got), pushed)
	}
	// nothing new → nothing new
	*w.now = w.now.Add(4 * time.Hour)
	w.srv.watch.runDue()
	if len(w.events(t)) != len(got) {
		t.Fatalf("repeated events: %d → %d", len(got), len(w.events(t)))
	}
}

func TestWatcherKeepsIntervalsAndNightPause(t *testing.T) {
	w := newWatchHarness(t, app.NotifySettings{})
	w.srv.watch.runDue()
	first := w.f.calls["cis_grades"]
	*w.now = w.now.Add(20 * time.Minute)
	w.srv.watch.runDue()
	if w.f.calls["cis_grades"] != first || w.f.calls["moodle_conversations"] != 2 {
		t.Fatalf("intervals: grades %d→%d, conversations %d", first, w.f.calls["cis_grades"], w.f.calls["moodle_conversations"])
	}
	*w.now = time.Date(2026, 10, 3, 1, 0, 0, 0, time.UTC) // 03:00 in Berlin
	before := w.f.calls["moodle_conversations"]
	w.srv.watch.runDue()
	if w.f.calls["moodle_conversations"] != before {
		t.Fatal("watcher ran at night")
	}
}

func TestWatcherOff(t *testing.T) {
	w := newWatchHarness(t, app.NotifySettings{Off: true})
	w.srv.watch.runDue()
	if len(w.f.calls) != 0 {
		t.Fatalf("calls while off: %v", w.f.calls)
	}
}

func TestNotificationsAPI(t *testing.T) {
	w := newWatchHarness(t, app.NotifySettings{})
	w.srv.watch.add(Notification{Kind: "grade", Title: "Neue Note: X 1,0", URL: "#/noten"})
	srv := httptest.NewServer(w.srv.Handler())
	t.Cleanup(srv.Close)
	get := func() map[string]any {
		req, _ := http.NewRequest("GET", srv.URL+"/api/notifications", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		json.NewDecoder(res.Body).Decode(&m)
		return m
	}
	m := get()
	if m["unread"] != float64(1) {
		t.Fatalf("unread: %v", m)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/api/notifications/read", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, _ := http.DefaultClient.Do(req)
	res.Body.Close()
	if m := get(); m["unread"] != float64(0) {
		t.Fatalf("after read: %v", m)
	}
}

func TestEventStreamDeliversNotifications(t *testing.T) {
	w := newWatchHarness(t, app.NotifySettings{})
	srv := httptest.NewServer(w.srv.Handler())
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest("GET", srv.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content type %q", res.Header.Get("Content-Type"))
	}
	var got atomic.Value
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		var all string
		for {
			n, err := res.Body.Read(buf)
			all += string(buf[:n])
			if strings.Contains(all, "Neue Note") || err != nil {
				got.Store(all)
				close(done)
				return
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	w.srv.watch.add(Notification{Kind: "grade", Title: "Neue Note: Y 2,0"})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("no event on the stream")
	}
	if s, _ := got.Load().(string); !strings.Contains(s, "event: notification") {
		t.Fatalf("stream: %q", s)
	}
}
