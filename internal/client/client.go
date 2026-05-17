package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	BaseURL   = "https://cis.nordakademie.de"
	UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

type Client struct {
	HTTP       *http.Client
	jar        *cookiejar.Jar
	sessionDir string
}

func New() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}

	httpClient := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			req.Header.Set("User-Agent", UserAgent)
			return nil
		},
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	sessionDir := filepath.Join(home, ".config", "cis-api")

	c := &Client{
		HTTP:       httpClient,
		jar:        jar,
		sessionDir: sessionDir,
	}
	_ = c.loadSession() // ignore error — fresh login if no session
	return c, nil
}

func (c *Client) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	return c.HTTP.Do(req)
}

func (c *Client) GetURL(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	return c.HTTP.Do(req)
}

func (c *Client) PostForm(path string, data url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", BaseURL)
	req.Header.Set("Referer", BaseURL+"/")
	req.URL.RawQuery = ""
	encoded := data.Encode()
	req.ContentLength = int64(len(encoded))
	req.Body = io.NopCloser(newStringReader(encoded))
	return c.HTTP.Do(req)
}

func (c *Client) PostFormURL(rawURL string, data url.Values) (*http.Response, error) {
	encoded := data.Encode()
	req, err := http.NewRequest(http.MethodPost, rawURL, newStringReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", BaseURL)
	req.Header.Set("Referer", BaseURL+"/")
	req.ContentLength = int64(len(encoded))
	return c.HTTP.Do(req)
}

func (c *Client) IsLoggedIn() bool {
	u, _ := url.Parse(BaseURL)
	for _, cookie := range c.jar.Cookies(u) {
		if cookie.Name == "fe_typo_user_cae070b" && cookie.Value != "" {
			return true
		}
	}
	return false
}

type savedCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (c *Client) SaveSession() error {
	if err := os.MkdirAll(c.sessionDir, 0700); err != nil {
		return err
	}
	u, _ := url.Parse(BaseURL)
	cookies := c.jar.Cookies(u)
	var saved []savedCookie
	for _, ck := range cookies {
		saved = append(saved, savedCookie{Name: ck.Name, Value: ck.Value})
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.sessionDir, "session.json"), data, 0600)
}

func (c *Client) loadSession() error {
	data, err := os.ReadFile(filepath.Join(c.sessionDir, "session.json"))
	if err != nil {
		return err
	}
	var saved []savedCookie
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	u, _ := url.Parse(BaseURL)
	var cookies []*http.Cookie
	for _, s := range saved {
		cookies = append(cookies, &http.Cookie{
			Name:   s.Name,
			Value:  s.Value,
			Domain: "cis.nordakademie.de",
			Path:   "/",
		})
	}
	c.jar.SetCookies(u, cookies)
	return nil
}

func (c *Client) ClearSession() {
	os.Remove(filepath.Join(c.sessionDir, "session.json"))
}

// stringReader wraps a string as an io.Reader without buffering
type stringReader struct {
	s   string
	pos int
}

func newStringReader(s string) *stringReader { return &stringReader{s: s} }
func (r *stringReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.pos:])
	r.pos += n
	return n, nil
}
