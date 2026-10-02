package cmd

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBinaryServesWebUI starts the real binary the way the Docker image does
// and checks the start-up contract: health endpoint, generated token, login
// link in the log, authenticated API and a clean shutdown on SIGTERM.
func TestBinaryServesWebUI(t *testing.T) {
	bin := buildBinary(t)
	data := t.TempDir()
	cmd := exec.Command(bin, "serve", "--addr", "127.0.0.1:0")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "NAK_DATA_DIR="+data, "NAK_WEB_TOKEN=",
		"CIS_USER=", "CIS_PASS=", "MOODLE_USER=", "MOODLE_PASS=", "NAK_READONLY=1")
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	linkRe := regexp.MustCompile(`(http://127\.0\.0\.1:\d+)/login#token=(\S+)`)
	found := make(chan []string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := linkRe.FindStringSubmatch(sc.Text()); m != nil {
				found <- m
			}
		}
	}()
	var base, token string
	select {
	case m := <-found:
		base, token = m[1], m[2]
	case <-time.After(15 * time.Second):
		t.Fatal("no login link in the log")
	}

	b, err := os.ReadFile(filepath.Join(data, "web-token"))
	if err != nil || strings.TrimSpace(string(b)) != token {
		t.Fatalf("token file %q vs log %q (%v)", b, token, err)
	}

	res, err := http.Get(base + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", res, err)
	}
	res.Body.Close()

	get := func(path string) *http.Response {
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	var list struct {
		Tools    []map[string]any `json:"tools"`
		ReadOnly bool             `json:"read_only"`
	}
	json.NewDecoder(get("/api/tools").Body).Decode(&list)
	if len(list.Tools) < 75 || !list.ReadOnly {
		t.Fatalf("api/tools: %d tools, read_only=%v", len(list.Tools), list.ReadOnly)
	}

	// A tool without credentials fails as JSON, the server keeps running.
	req, _ := http.NewRequest("POST", base+"/api/tools/moodle_whoami", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusBadGateway {
		t.Fatalf("call without credentials: %v %v", res, err)
	}
	res.Body.Close()

	hc := exec.Command(bin, "healthcheck", "--url", base+"/healthz")
	if out, err := hc.CombinedOutput(); err != nil {
		t.Fatalf("healthcheck: %v %s", err, out)
	}

	// an open live stream (SSE) must not hold up the shutdown
	evReq, _ := http.NewRequest("GET", base+"/api/events", nil)
	evReq.Header.Set("Authorization", "Bearer "+token)
	evRes, err := http.DefaultClient.Do(evReq)
	if err != nil || evRes.StatusCode != 200 {
		t.Fatalf("events: %v %v", evRes, err)
	}
	defer evRes.Body.Close()

	stopAt := time.Now()
	cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit after SIGTERM: %v", err)
		}
		if d := time.Since(stopAt); d > 3*time.Second {
			t.Fatalf("shutdown took %v (docker stop kills after 10 s)", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no shutdown after SIGTERM")
	}

	if out, err := exec.Command(bin, "healthcheck", "--url", base+"/healthz").CombinedOutput(); err == nil {
		t.Fatalf("healthcheck succeeded against a stopped server: %s", out)
	}

	// Second start with the stored token: the log says where it is but does
	// not repeat it.
	again := exec.Command(bin, "serve", "--addr", "127.0.0.1:0")
	again.Env = cmd.Env
	out, _ := again.StderrPipe()
	if err := again.Start(); err != nil {
		t.Fatal(err)
	}
	defer again.Process.Kill()
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, token) {
			t.Fatalf("token repeated in the log: %s", line)
		}
		if strings.Contains(line, "web-token") {
			break
		}
	}
}

// TestDemoNeedsNoAccount: `serve --demo` answers every UI tool with invented
// data and needs neither a login nor the CIS.
func TestDemoNeedsNoAccount(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "serve", "--demo", "--addr", "127.0.0.1:0")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "CIS_USER=", "CIS_PASS=", "CIS_BASE_URL=http://127.0.0.1:1")
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	re := regexp.MustCompile(`web UI on (http://127\.0\.0\.1:\d+)`)
	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				found <- m[1]
			}
		}
	}()
	var base string
	select {
	case base = <-found:
	case <-time.After(15 * time.Second):
		t.Fatal("demo did not start")
	}
	for _, tool := range []string{"nak_dashboard", "nak_agenda", "nak_deadlines", "cis_grades", "cis_progress", "cis_list_klausuren", "moodle_courses", "moodle_whoami", "moodle_whats_new", "moodle_conversations", "moodle_assignments", "eduvault_module_exams"} {
		res, err := http.Post(base+"/api/tools/"+tool, "application/json", strings.NewReader(`{"title":"Datenbanksysteme"}`))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("%s: %d", tool, res.StatusCode)
		}
	}
	// a binding action only ever answers "demo"
	req, _ := http.NewRequest("POST", base+"/api/tools/cis_klausur_action", strings.NewReader(`{"exam_id":"9002","action":"register","confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nak-Confirm", "JA")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.NewDecoder(res.Body).Decode(&m)
	res.Body.Close()
	if b, _ := json.Marshal(m); !strings.Contains(string(b), "Demo") {
		t.Fatalf("demo write: %s", b)
	}
}
