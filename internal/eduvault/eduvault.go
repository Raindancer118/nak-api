// Package eduvault reads old exams from EduVault (eduvault4.de). It logs in
// the way the EduVault MCP client does: a long-lived credential token plus a
// fresh code from the credential's own TOTP secret buy a short Bearer session.
package eduvault

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultURL = "https://eduvault4.de"

var (
	ErrNotConfigured = errors.New("EduVault ist nicht eingerichtet (Token und TOTP-Secret in den Einstellungen hinterlegen)")
	ErrAuth          = errors.New("EduVault-Anmeldung fehlgeschlagen")
)

// TOTP is RFC 6238 with SHA1, 30 s steps and 6 digits — what EduVault issues.
func TOTP(secret string, at time.Time) (string, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(s, "="))
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("TOTP-Secret ist kein gültiges Base32")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000), nil
}

type Client struct {
	Base, Token, Secret string
	HTTP                *http.Client
	Now                 func() time.Time

	mu      sync.Mutex
	session string
	expires time.Time
}

func New(base, token, secret string) *Client {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		b = DefaultURL
	}
	return &Client{Base: b, Token: strings.TrimSpace(token), Secret: strings.TrimSpace(secret), HTTP: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Login mints a session (also used to check credentials before saving them).
func (c *Client) Login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked()
}

func (c *Client) loginLocked() error {
	if c.Token == "" || c.Secret == "" {
		return ErrNotConfigured
	}
	code, err := TOTP(c.Secret, c.now())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuth, err)
	}
	body, _ := json.Marshal(map[string]string{"token": c.Token, "code": code})
	req, err := http.NewRequest(http.MethodPost, c.Base+"/api/mcp/session", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nak-api")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return fmt.Errorf("%w: %s", ErrAuth, e.Error)
	}
	var s struct {
		Session   string `json:"session"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &s); err != nil || s.Session == "" {
		return fmt.Errorf("%w: Antwort ohne Session", ErrAuth)
	}
	if s.ExpiresIn <= 0 {
		s.ExpiresIn = 3600
	}
	c.session = s.Session
	// renew a minute early so a session never expires mid-request
	c.expires = c.now().Add(time.Duration(s.ExpiresIn)*time.Second - time.Minute)
	return nil
}

func (c *Client) bearer(force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if force || c.session == "" || !c.now().Before(c.expires) {
		if err := c.loginLocked(); err != nil {
			return "", err
		}
	}
	return c.session, nil
}

func (c *Client) get(path string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		tok, err := c.bearer(attempt > 0)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest(http.MethodGet, c.Base+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", "nak-api")
		res, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusUnauthorized && attempt == 0 {
			res.Body.Close()
			continue // session revoked or expired server-side: one silent relogin
		}
		if res.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<12))
			res.Body.Close()
			return nil, fmt.Errorf("EduVault %s: HTTP %d %s", path, res.StatusCode, strings.TrimSpace(string(b)))
		}
		return res, nil
	}
}

type Query struct {
	Q, Modul, Dozent, Jahr, Studiengang string
	Types                               []string
	Limit                               int
}

type Result struct {
	Results []map[string]any `json:"results"`
	Total   int              `json:"total"`
}

func (c *Client) Search(q Query) (*Result, error) {
	v := url.Values{}
	for k, x := range map[string]string{"q": q.Q, "modul": q.Modul, "dozent": q.Dozent, "jahr": q.Jahr, "studiengang": q.Studiengang} {
		if strings.TrimSpace(x) != "" {
			v.Set(k, strings.TrimSpace(x))
		}
	}
	for _, t := range q.Types {
		v.Add("type", t)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	res, err := c.get("/api/storage/search?" + v.Encode())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var r Result
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("EduVault-Suche: %w", err)
	}
	return &r, nil
}

var trailingNum = regexp.MustCompile(`^(.*?)\s*(\d{1,2})$`)
var trailingRoman = regexp.MustCompile(`^(.*\S)\s+(I{1,3}|IV|VI{0,3}|IX|X)$`)
var romans = []string{"", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}

// Spellings lists the ways a module title appears in EduVault: modules are
// uploaded as "Mathematik 2" as well as "Mathematik II".
func Spellings(title string) []string {
	t := strings.TrimSpace(title)
	out := []string{t}
	add := func(s string) {
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	if m := trailingNum.FindStringSubmatch(t); m != nil {
		if n, _ := strconv.Atoi(m[2]); n >= 1 && n <= 10 && strings.TrimSpace(m[1]) != "" {
			base := strings.TrimSpace(m[1])
			add(base + " " + m[2])
			add(base + " " + romans[n])
		}
	} else if m := trailingRoman.FindStringSubmatch(t); m != nil {
		for n, r := range romans {
			if r == m[2] {
				add(m[1] + " " + strconv.Itoa(n))
			}
		}
	}
	return out
}

// ForModule collects old exams and practice exams for a module title over all
// its spellings, newest year first.
func (c *Client) ForModule(title string, types ...string) ([]map[string]any, error) {
	if len(types) == 0 {
		types = []string{"exam", "probeklausur"}
	}
	seen := map[string]bool{}
	var out []map[string]any
	var firstErr error
	for _, s := range Spellings(title) {
		r, err := c.Search(Query{Modul: s, Types: types, Limit: 100})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, x := range r.Results {
			id := fmt.Sprint(x["id"])
			if !seen[id] {
				seen[id] = true
				out = append(out, x)
			}
		}
	}
	if out == nil && firstErr != nil {
		return nil, firstErr
	}
	sort.SliceStable(out, func(i, j int) bool { return fmt.Sprint(out[i]["jahr"]) > fmt.Sprint(out[j]["jahr"]) })
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// File downloads one document; only UUIDs are accepted as ids.
func (c *Client) File(id string) (data []byte, contentType, filename string, err error) {
	if !uuidRe.MatchString(id) {
		return nil, "", "", fmt.Errorf("ungültige EduVault-id %q", id)
	}
	res, err := c.get("/api/storage/file/" + id + "?view=true")
	if err != nil {
		return nil, "", "", err
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, 128<<20))
	if err != nil {
		return nil, "", "", err
	}
	if _, p, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil {
		filename = p["filename"]
	}
	return data, res.Header.Get("Content-Type"), filename, nil
}
