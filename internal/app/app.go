// Package app wires the CIS and Moodle clients from the environment.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Raindancer118/nak-api/internal/auth"
	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/moodle"
)

// App is the shared context of all tools. Clients are created lazily so the
// MCP server starts instantly and works offline until a tool needs a system.
type App struct {
	Zone        *time.Location
	Now         func() time.Time
	DownloadDir string
	ConfigDir   string
	ReadOnly    bool
	// ClientCache shares identical CIS/Moodle reads for this long.
	ClientCache time.Duration

	cisUser, cisPass       string
	moodleUser, moodlePass string
	moodleURL              string
	cisBase                string

	mu     sync.Mutex
	cis    *client.Client
	moodle *moodle.Service
}

// FromEnv reads CIS_USER/CIS_PASS, MOODLE_USER/MOODLE_PASS (falling back to
// the CIS account — both use the NAK login), NAK_READONLY, NAK_DOWNLOAD_DIR,
// MOODLE_URL, CIS_BASE_URL and NAK_TZ. NAK_DATA_DIR moves the session, audit
// log and (unless NAK_DOWNLOAD_DIR is set) the downloads into one directory.
func FromEnv() *App {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "cis-api")
	downloadDir := filepath.Join(home, "Downloads", "nak")
	if d := strings.TrimSpace(os.Getenv("NAK_DATA_DIR")); d != "" {
		configDir, downloadDir = d, filepath.Join(d, "downloads")
	}
	zone, err := time.LoadLocation(envOr("NAK_TZ", "Europe/Berlin"))
	if err != nil {
		zone = time.UTC
	}
	a := &App{
		Zone:        zone,
		Now:         time.Now,
		DownloadDir: envOr("NAK_DOWNLOAD_DIR", downloadDir),
		ConfigDir:   configDir,
		ReadOnly:    truthy(os.Getenv("NAK_READONLY")) || truthy(os.Getenv("CIS_READONLY")),
		cisUser:     os.Getenv("CIS_USER"),
		cisPass:     os.Getenv("CIS_PASS"),
		moodleUser:  envOr("MOODLE_USER", os.Getenv("CIS_USER")),
		moodlePass:  envOr("MOODLE_PASS", os.Getenv("CIS_PASS")),
		moodleURL:   envOr("MOODLE_URL", moodle.DefaultURL),
		cisBase:     os.Getenv("CIS_BASE_URL"),
		ClientCache: 90 * time.Second,
	}
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("NAK_CLIENT_CACHE_TTL"))); err == nil {
		a.ClientCache = d
	}
	return a
}

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "ja":
		return true
	}
	return false
}

// UseCIS / UseMoodle inject clients (tests, custom hosts).
func (a *App) UseCIS(c *client.Client) {
	a.mu.Lock()
	a.cis = c
	a.mu.Unlock()
}

func (a *App) UseMoodle(s *moodle.Service) {
	a.mu.Lock()
	a.moodle = s
	a.mu.Unlock()
}

// CIS returns the logged-in CIS client, logging in from the environment when
// no valid session cookie exists.
func (a *App) CIS() (*client.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cis == nil {
		c, err := client.NewWith(client.Options{BaseURL: a.cisBase, SessionDir: a.ConfigDir})
		if err != nil {
			return nil, err
		}
		c.ReadOnly = a.ReadOnly
		c.CacheTTL, c.Now = a.ClientCache, a.Now
		if a.cisUser != "" && a.cisPass != "" {
			u, p := a.cisUser, a.cisPass
			c.Relogin = func(c *client.Client) error { return auth.Login(c, u, p) }
		}
		a.cis = c
	}
	if !a.cis.IsLoggedIn() {
		if a.cis.Relogin == nil {
			return nil, client.ErrNotLoggedIn
		}
		if err := a.cis.Relogin(a.cis); err != nil {
			return nil, fmt.Errorf("CIS login: %w", err)
		}
	}
	return a.cis, nil
}

// CISClient returns the client without forcing a login (for cis_login/status).
func (a *App) CISClient() (*client.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cis == nil {
		c, err := client.NewWith(client.Options{BaseURL: a.cisBase, SessionDir: a.ConfigDir})
		if err != nil {
			return nil, err
		}
		c.ReadOnly = a.ReadOnly
		c.CacheTTL, c.Now = a.ClientCache, a.Now
		a.cis = c
	}
	return a.cis, nil
}

func (a *App) Moodle() (*moodle.Service, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.moodle == nil {
		if a.moodleUser == "" || a.moodlePass == "" {
			return nil, fmt.Errorf("Moodle credentials missing: set MOODLE_USER/MOODLE_PASS (or CIS_USER/CIS_PASS)")
		}
		mc := moodle.NewClient(a.moodleURL, a.moodleUser, a.moodlePass)
		mc.ReadOnly = a.ReadOnly
		mc.AuditDir = a.ConfigDir
		mc.CacheTTL, mc.Now = a.ClientCache, a.Now
		s := moodle.NewService(mc, a.Zone)
		s.Now = a.Now
		a.moodle = s
	}
	return a.moodle, nil
}

// ExpandPath resolves "~/" and relative paths (relative to DownloadDir).
func (a *App) ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	if p != "" && !filepath.IsAbs(p) {
		return filepath.Join(a.DownloadDir, p)
	}
	return p
}
