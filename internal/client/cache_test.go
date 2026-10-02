package client

import (
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadsAreSharedForAShortTime(t *testing.T) {
	var hits atomic.Int32
	c, srv := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		io.WriteString(w, "<main>page "+r.URL.Path+"</main>")
	})
	now := time.Unix(1000, 0)
	c.CacheTTL, c.Now = time.Minute, func() time.Time { return now }

	for i := 0; i < 3; i++ {
		p, err := c.Page("/a")
		if err != nil || p.Body != "<main>page /a</main>" {
			t.Fatalf("page: %v %q", err, p.Body)
		}
	}
	c.PostRead("/a", url.Values{"x": {"1"}})
	c.PostRead("/a", url.Values{"x": {"1"}})
	c.PostRead("/a", url.Values{"x": {"2"}})
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 3 (one GET, two distinct POST bodies)", hits.Load())
	}
	now = now.Add(61 * time.Second)
	c.Page("/a")
	if hits.Load() != 4 {
		t.Fatalf("expired entry reused: hits = %d", hits.Load())
	}
	// a write invalidates everything read before it
	c.WritePost(srv.URL+"/w", url.Values{"a": {"1"}})
	c.Page("/a")
	if hits.Load() != 6 {
		t.Fatalf("cache survived a write: hits = %d", hits.Load())
	}
}

func TestErrorsAndLoginPagesAreNotCached(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	c.CacheTTL = time.Minute
	c.Page("/a")
	c.Page("/a")
	if hits.Load() != 2 {
		t.Fatalf("error page cached: hits = %d", hits.Load())
	}
}

func TestDownloadsBypassTheCache(t *testing.T) {
	var hits atomic.Int32
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/pdf")
		io.WriteString(w, "%PDF")
	})
	c.CacheTTL = time.Minute
	c.Download("/f.pdf")
	c.Download("/f.pdf")
	if hits.Load() != 2 {
		t.Fatalf("download cached: hits = %d", hits.Load())
	}
}

// Parallel reads that all find the session expired must log in once.
func TestConcurrentReadsReloginOnce(t *testing.T) {
	var mu sync.Mutex
	loggedIn := false
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ok := loggedIn
		mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `<form><input type="password" name="pass"></form>`)
			return
		}
		io.WriteString(w, `<main>ok</main><a href="/login/?logintype=logout">x</a>`)
	})
	var logins atomic.Int32
	c.Relogin = func(*Client) error {
		logins.Add(1)
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		loggedIn = true
		mu.Unlock()
		return nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := c.Page("/p" + string(rune('a'+i))); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if logins.Load() != 1 {
		t.Fatalf("logins = %d", logins.Load())
	}
}
