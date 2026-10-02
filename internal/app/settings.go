package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Raindancer118/nak-api/internal/eduvault"
)

// Settings are what the owner changes in the web UI. They live in
// settings.json in the data dir (0600 — they hold the EduVault credential).
type Settings struct {
	EduVault EduVaultSettings `json:"eduvault"`
	Notify   NotifySettings   `json:"notify"`
	// Nav is the owner's choice and order of navigation entries (ids from
	// NavItems); empty = DefaultNav.
	Nav []string `json:"nav,omitempty"`
	// Home is the choice and order of dashboard tiles (ids from HomeItems).
	Home []string `json:"home,omitempty"`
}

// NavItems are the pages the navigation bar can hold; the web UI knows
// their labels and icons.
var NavItems = []string{"start", "woche", "kurse", "inbox", "noten", "studium", "pruefungen", "abgaben", "transfer"}

var DefaultNav = []string{"start", "woche", "kurse", "inbox", "noten", "studium"}

// HomeItems are the dashboard tiles; the web UI renders them.
var HomeItems = []string{"next", "deadlines", "week", "grades", "exams", "moodle", "messages", "pending", "transfer", "transfer_grades"}

var DefaultHome = []string{"next", "deadlines", "week", "grades", "exams", "moodle"}

func (s Settings) HomeOrDefault() []string {
	if len(s.Home) == 0 {
		return DefaultHome
	}
	return s.Home
}

// NavOrDefault is the bar as it should be shown.
func (s Settings) NavOrDefault() []string {
	if len(s.Nav) == 0 {
		return DefaultNav
	}
	return s.Nav
}

// NotifySettings control the background watcher (new grades, Moodle news,
// messages, urgent deadlines).
type NotifySettings struct {
	// Off disables the watcher (zero value = on, so it works out of the box).
	Off bool `json:"off,omitempty"`
	// NtfyURL, if set, gets every notification as a push (https://ntfy.sh/<topic>
	// or a self-hosted ntfy).
	NtfyURL string `json:"ntfy_url,omitempty"`
	// NtfyDetails sends titles like "Neue Note: Datenbanken 1,7" instead of a
	// generic "Neue Note in naknak" — off by default, ntfy.sh is a public server.
	NtfyDetails bool `json:"ntfy_details,omitempty"`
	// Night lets the watcher run between 23:00 and 07:00 too.
	Night bool `json:"night,omitempty"`
	// FastGrades also reads the Notenübersicht PDF every 10 minutes while a
	// grade is pending: it knows new grades before the Leistungsübersicht.
	FastGrades bool `json:"fast_grades,omitempty"`
}

type EduVaultSettings struct {
	URL    string `json:"url,omitempty"`
	Token  string `json:"token,omitempty"`
	Secret string `json:"secret,omitempty"`
}

func (e EduVaultSettings) set() bool { return e.Token != "" && e.Secret != "" }

func (a *App) settingsFile() string { return filepath.Join(a.ConfigDir, "settings.json") }

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settingsLocked()
}

func (a *App) settingsLocked() Settings {
	if a.settings != nil {
		return *a.settings
	}
	var s Settings
	if b, err := os.ReadFile(a.settingsFile()); err == nil {
		json.Unmarshal(b, &s)
	}
	a.settings = &s
	return s
}

func (a *App) SaveSettings(s Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(a.ConfigDir, 0o700); err != nil {
		return err
	}
	tmp := a.settingsFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, a.settingsFile()); err != nil {
		return err
	}
	a.mu.Lock()
	a.settings = &s
	a.eduvault = nil
	a.mu.Unlock()
	return nil
}

func (a *App) eduVaultConfig() (EduVaultSettings, string) {
	if s := a.settingsLocked().EduVault; s.set() {
		return s, "settings"
	}
	// same variables as the EduVault MCP, so an existing setup just works
	env := EduVaultSettings{URL: os.Getenv("EDUVAULT_URL"), Token: strings.TrimSpace(os.Getenv("EDUVAULT_TOKEN")), Secret: strings.TrimSpace(os.Getenv("EDUVAULT_MCP_SECRET"))}
	if env.set() {
		return env, "env"
	}
	return EduVaultSettings{}, ""
}

// EduVaultSource is "settings", "env" or "" (not configured).
func (a *App) EduVaultSource() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, src := a.eduVaultConfig()
	return src
}

func (a *App) EduVault() (*eduvault.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.eduvault != nil {
		return a.eduvault, nil
	}
	cfg, src := a.eduVaultConfig()
	if src == "" {
		return nil, eduvault.ErrNotConfigured
	}
	a.eduvault = eduvault.New(cfg.URL, cfg.Token, cfg.Secret)
	return a.eduvault, nil
}

// ErrEduVault reports whether err means EduVault is simply not set up.
func ErrEduVault(err error) bool { return errors.Is(err, eduvault.ErrNotConfigured) }

// ClearAccount forgets the stored NAK login (instance reset). An account
// from CIS_USER/CIS_PASS cannot be removed here.
func (a *App) ClearAccount() {
	os.Remove(filepath.Join(a.ConfigDir, "account.json"))
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.accountSource == "file" {
		a.cisUser, a.cisPass, a.accountSource = "", "", ""
		if os.Getenv("MOODLE_USER") == "" {
			a.moodleUser, a.moodlePass = "", ""
		}
	}
	if a.cis != nil {
		a.cis.ClearSession()
	}
	a.cis, a.moodle, a.eduvault, a.settings = nil, nil, nil, nil
}
