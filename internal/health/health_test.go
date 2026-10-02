package health

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Raindancer118/nak-api/internal/drift"
)

func driftResult(check string) Result {
	fp := drift.Fingerprint(`<main><table><tr><th>Modulnummer</th></tr><tr><td data-label="Note:">1,0 Max Mustermann</td></tr></table></main>`)
	return Result{Check: check, Status: StatusDrift, Page: "/x", Expected: "table", Print: &fp}
}

func TestBodyAndLinkCarryNoContent(t *testing.T) {
	r := driftResult("grades overview")
	b := Body(r, "0.1.0", time.Unix(0, 0))
	if !strings.Contains(b, "Modulnummer") || strings.Contains(b, "Mustermann") || strings.Contains(b, "1,0") {
		t.Errorf("body:\n%s", b)
	}
	u, _ := url.Parse(IssueURL(DefaultRepo, r, "0.1.0", time.Unix(0, 0)))
	if u.Host != "github.com" || u.Query().Get("labels") != Label || u.Query().Get("title") != "CIS changed: grades overview" {
		t.Errorf("url = %s", u)
	}
}

func TestSyncCreatesCommentsAndCloses(t *testing.T) {
	var calls []string
	open := []map[string]any{
		{"number": 7, "title": "CIS changed: seminars", "html_url": "https://gh/7"},
		{"number": 8, "title": "CIS changed: study plan", "html_url": "https://gh/8"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(b))
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "GET":
			json.NewEncoder(w).Encode(open)
		case r.URL.Path == "/repos/o/r/issues":
			w.Write([]byte(`{"number":9,"html_url":"https://gh/9"}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	rep := &Report{Version: "t", At: time.Unix(0, 0), Results: []Result{
		driftResult("grades overview"),                                        // new → create
		driftResult("seminars"),                                               // already open → comment
		{Check: "study plan", Status: StatusOK},                               // healthy again → close
		{Check: "certificates", Status: StatusUnavailable, Detail: "timeout"}, // ignored
	}}
	acts, err := (&GitHub{API: srv.URL, Repo: "o/r", Token: "tok"}).Sync(rep)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, a := range acts {
		got = append(got, a.Check+":"+a.Action)
	}
	if strings.Join(got, ",") != "grades overview:created,seminars:commented,study plan:closed" {
		t.Fatalf("actions = %v\ncalls = %v", got, calls)
	}
	for _, c := range calls {
		if strings.Contains(c, "Mustermann") || strings.Contains(c, "certificates") {
			t.Errorf("leak or unexpected call: %s", c)
		}
	}
}
