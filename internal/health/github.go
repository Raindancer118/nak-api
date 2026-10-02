package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	DefaultRepo = "Raindancer118/nak-api"
	Label       = "cis-drift"
)

func Title(r Result) string { return "CIS changed: " + r.Check }

// Body is the public issue text: what broke and the page outline, no content.
func Body(r Result, version string, at time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "nak `%s` noticed that the CIS no longer looks the way the parser for **%s** expects.\n\n", version, r.Check)
	fmt.Fprintf(&b, "- **page:** `%s`\n- **expected:** %s\n- **seen:** %s\n\n", r.Page, r.Expected, at.UTC().Format(time.RFC3339))
	if r.Print != nil {
		b.WriteString("<details><summary>Page outline (structure only, no content)</summary>\n\n")
		b.WriteString(r.Print.String())
		b.WriteString("\n</details>\n\n")
	}
	b.WriteString("_Reported by `nak selfcheck`. The outline lists table headers, data-labels, form field names and CSS classes only; it never contains cell values, headings or personal data._\n")
	return b.String()
}

// IssueURL is a pre-filled "new issue" link for people without a token.
func IssueURL(repo string, r Result, version string, at time.Time) string {
	q := url.Values{"title": {Title(r)}, "labels": {Label}, "body": {Body(r, version, at)}}
	u := "https://github.com/" + repo + "/issues/new?" + q.Encode()
	if len(u) > 7500 { // browsers and GitHub cap URL length
		q.Set("body", Body(Result{Check: r.Check, Page: r.Page, Expected: r.Expected}, version, at)+"\n_(outline omitted: too long for a link — run `nak selfcheck`)_")
		u = "https://github.com/" + repo + "/issues/new?" + q.Encode()
	}
	return u
}

// Token: NAK_GITHUB_TOKEN, GITHUB_TOKEN, or the gh CLI login.
func Token() string {
	for _, k := range []string{"NAK_GITHUB_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

type GitHub struct {
	API   string // https://api.github.com
	Repo  string
	Token string
	HTTP  *http.Client
}

type issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"html_url"`
}

type Action struct {
	Check  string `json:"check"`
	Action string `json:"action"` // created, commented, closed
	URL    string `json:"url"`
}

func (g *GitHub) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(g.API, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	hc := g.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub %s %s: HTTP %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(b))[:min(200, len(strings.TrimSpace(string(b))))])
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// Sync opens one issue per drifted check (or comments on the open one) and
// closes open issues of checks that pass again. Unavailable checks are left alone.
func (g *GitHub) Sync(r *Report) ([]Action, error) {
	var open []issue
	if err := g.do("GET", "/repos/"+g.Repo+"/issues?state=open&labels="+Label+"&per_page=100", nil, &open); err != nil {
		return nil, err
	}
	byTitle := map[string]issue{}
	for _, i := range open {
		byTitle[i.Title] = i
	}
	var acts []Action
	for _, res := range r.Results {
		existing, has := byTitle[Title(res)]
		switch {
		case res.Status == StatusDrift && !has:
			var created issue
			if err := g.do("POST", "/repos/"+g.Repo+"/issues", map[string]any{"title": Title(res), "body": Body(res, r.Version, r.At), "labels": []string{Label}}, &created); err != nil {
				return acts, err
			}
			acts = append(acts, Action{res.Check, "created", created.URL})
		case res.Status == StatusDrift && has:
			if err := g.do("POST", fmt.Sprintf("/repos/%s/issues/%d/comments", g.Repo, existing.Number),
				map[string]string{"body": "Still broken as of " + r.At.UTC().Format(time.RFC3339) + " (nak `" + r.Version + "`)."}, nil); err != nil {
				return acts, err
			}
			acts = append(acts, Action{res.Check, "commented", existing.URL})
		case res.Status == StatusOK && has:
			if err := g.do("POST", fmt.Sprintf("/repos/%s/issues/%d/comments", g.Repo, existing.Number),
				map[string]string{"body": "`nak selfcheck` passes again for this check (" + r.At.UTC().Format(time.RFC3339) + ", nak `" + r.Version + "`). Closing."}, nil); err != nil {
				return acts, err
			}
			if err := g.do("PATCH", fmt.Sprintf("/repos/%s/issues/%d", g.Repo, existing.Number), map[string]string{"state": "closed", "state_reason": "completed"}, nil); err != nil {
				return acts, err
			}
			acts = append(acts, Action{res.Check, "closed", existing.URL})
		}
	}
	return acts, nil
}
