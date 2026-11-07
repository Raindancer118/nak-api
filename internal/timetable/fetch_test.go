package timetable

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/stundenplan"
)

func TestFetchWithoutOwnPlanIsErrNoPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != stundenplan.PagePath {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><ul class="ce-uploads"><li><a href="/x/I24a_5.ics"><span class="ce-uploads-fileName">I24a_5.ics</span></a></li></ul></body></html>`))
	}))
	defer srv.Close()
	c, err := client.NewWith(client.Options{BaseURL: srv.URL, SessionDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Fetch(c, "I23a")
	if !errors.Is(err, ErrNoPlan) {
		t.Fatalf("err = %v, want ErrNoPlan", err)
	}
}
