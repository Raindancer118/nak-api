package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// stats makes the cache's effect visible: how often naknak really asked the
// CIS/Moodle and how often a cached answer was enough.
type stats struct {
	mu    sync.Mutex
	since time.Time
	tools map[string]*toolStats
}

type toolStats struct {
	Fetches int `json:"fetches"`
	Hits    int `json:"hits"`
	Errors  int `json:"errors"`
}

func newStats(now time.Time) *stats { return &stats{since: now, tools: map[string]*toolStats{}} }

func (st *stats) get(tool string) *toolStats {
	t := st.tools[tool]
	if t == nil {
		t = &toolStats{}
		st.tools[tool] = t
	}
	return t
}

func (st *stats) hit(tool string)   { st.mu.Lock(); st.get(tool).Hits++; st.mu.Unlock() }
func (st *stats) fetch(tool string) { st.mu.Lock(); st.get(tool).Fetches++; st.mu.Unlock() }
func (st *stats) fail(tool string)  { st.mu.Lock(); st.get(tool).Errors++; st.mu.Unlock() }

func (st *stats) snapshot() (since time.Time, fetches, hits, errors int, per map[string]toolStats) {
	st.mu.Lock()
	defer st.mu.Unlock()
	per = map[string]toolStats{}
	for k, v := range st.tools {
		per[k] = *v
		fetches += v.Fetches
		hits += v.Hits
		errors += v.Errors
	}
	return st.since, fetches, hits, errors, per
}

func (s *Server) statsAPI(w http.ResponseWriter, r *http.Request) {
	since, f, h, e, per := s.stats.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"since": since.Format(time.RFC3339), "fetches": f, "cache_hits": h, "errors": e, "tools": per})
}

// metrics: Prometheus text format, behind the same auth as the API.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	since, _, _, _, per := s.stats.snapshot()
	names := make([]string, 0, len(per))
	for k := range per {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	series := func(name, help string, val func(toolStats) int) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
		for _, n := range names {
			fmt.Fprintf(&b, "%s{tool=%q} %d\n", name, n, val(per[n]))
		}
	}
	series("naknak_upstream_fetches_total", "Tool calls that went to the CIS/Moodle/EduVault.", func(t toolStats) int { return t.Fetches })
	series("naknak_cache_hits_total", "Tool calls answered from the cache.", func(t toolStats) int { return t.Hits })
	series("naknak_upstream_errors_total", "Failed upstream tool calls.", func(t toolStats) int { return t.Errors })
	unread := 0
	for _, n := range s.watch.list() {
		if !n.Read {
			unread++
		}
	}
	fmt.Fprintf(&b, "# HELP naknak_notifications_unread Unread notifications.\n# TYPE naknak_notifications_unread gauge\nnaknak_notifications_unread %d\n", unread)
	fmt.Fprintf(&b, "# HELP naknak_start_time_seconds Start of the process.\n# TYPE naknak_start_time_seconds gauge\nnaknak_start_time_seconds %d\n", since.Unix())
	fmt.Fprintf(&b, "# HELP naknak_up 1 while naknak runs.\n# TYPE naknak_up gauge\nnaknak_up 1\n")
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(b.String()))
}

func (st *stats) reset(now time.Time) {
	st.mu.Lock()
	st.since, st.tools = now, map[string]*toolStats{}
	st.mu.Unlock()
}
