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
