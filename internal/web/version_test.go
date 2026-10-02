package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{{"0.3.0", "0.3.1", true}, {"0.3.0", "0.3.0", false}, {"0.3.1", "0.3.0", false}, {"0.10.0", "0.9.9", false}, {"0.9.0", "0.10.0", true}, {"0.1.0-dev", "0.3.0", false}, {"0.3.0", "1.0.0", true}} {
		if got := newer(c.latest, c.cur); got != c.want {
			t.Errorf("newer(%s, %s) = %v", c.latest, c.cur, got)
		}
	}
}

func TestVersionCheckIsCached(t *testing.T) {
	var calls atomic.Int32
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"tag_name":"9.9.10","html_url":"https://github.com/Raindancer118/nak-api/releases/tag/9.9.10"}`)
	}))
	defer gh.Close()
	h := newHarness(t)
	h.web.releaseURL = gh.URL
	for i := 0; i < 3; i++ {
		m := decode(t, h.do(t, "GET", "/api/version", "", bearer))
		if m["current"] != "9.9.9" || m["latest"] != "9.9.10" || m["update"] != true {
			t.Fatalf("version: %v", m)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("GitHub asked %d times", calls.Load())
	}
}
