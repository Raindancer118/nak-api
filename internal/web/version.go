package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultReleaseURL = "https://api.github.com/repos/Raindancer118/nak-api/releases/latest"

type versionCache struct {
	mu     sync.Mutex
	at     time.Time
	latest string
	url    string
}

// newer compares X.Y.Z; dev builds never get an update hint.
func newer(latest, current string) bool {
	if strings.Contains(current, "-") {
		return false
	}
	parse := func(v string) []int {
		var out []int
		for _, p := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			n, _ := strconv.Atoi(p)
			out = append(out, n)
		}
		return out
	}
	l, c := parse(latest), parse(current)
	for i := 0; i < 3; i++ {
		var a, b int
		if i < len(l) {
			a = l[i]
		}
		if i < len(c) {
			b = c[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

// version answers "is there a newer naknak?" — asked only when the settings
// page is open, at most every 6 hours.
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	vc := &s.versions
	vc.mu.Lock()
	if vc.latest == "" || s.cfg.Now().Sub(vc.at) > 6*time.Hour {
		url := s.releaseURL
		if url == "" {
			url = defaultReleaseURL
		}
		c := &http.Client{Timeout: 8 * time.Second}
		if req, err := http.NewRequest(http.MethodGet, url, nil); err == nil {
			req.Header.Set("Accept", "application/vnd.github+json")
			if res, err := c.Do(req); err == nil {
				var rel struct {
					Tag string `json:"tag_name"`
					URL string `json:"html_url"`
				}
				if res.StatusCode == http.StatusOK && json.NewDecoder(res.Body).Decode(&rel) == nil && rel.Tag != "" {
					vc.latest, vc.url, vc.at = rel.Tag, rel.URL, s.cfg.Now()
				}
				res.Body.Close()
			}
		}
	}
	latest, url := vc.latest, vc.url
	vc.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"current": s.cfg.Version, "latest": latest, "url": url, "update": latest != "" && newer(latest, s.cfg.Version)})
}
