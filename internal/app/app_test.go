package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirMovesSessionAndDownloads(t *testing.T) {
	t.Setenv("NAK_DATA_DIR", "/data")
	t.Setenv("NAK_DOWNLOAD_DIR", "")
	a := FromEnv()
	if a.ConfigDir != "/data" || a.DownloadDir != filepath.Join("/data", "downloads") {
		t.Fatalf("config=%s downloads=%s", a.ConfigDir, a.DownloadDir)
	}
	t.Setenv("NAK_DOWNLOAD_DIR", "/elsewhere")
	if a := FromEnv(); a.DownloadDir != "/elsewhere" {
		t.Fatalf("explicit download dir ignored: %s", a.DownloadDir)
	}
}

func TestDefaultDirsUnchanged(t *testing.T) {
	t.Setenv("NAK_DATA_DIR", "")
	t.Setenv("NAK_DOWNLOAD_DIR", "")
	t.Setenv("HOME", "/home/x")
	a := FromEnv()
	if a.ConfigDir != "/home/x/.config/cis-api" || a.DownloadDir != "/home/x/Downloads/nak" {
		t.Fatalf("config=%s downloads=%s", a.ConfigDir, a.DownloadDir)
	}
}

func TestSettingsArePrivateAndWinOverEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NAK_DATA_DIR", dir)
	t.Setenv("EDUVAULT_URL", "https://env.example")
	t.Setenv("EDUVAULT_TOKEN", "evm_env")
	t.Setenv("EDUVAULT_MCP_SECRET", "ENVSECRET")
	a := FromEnv()
	ev, err := a.EduVault()
	if err != nil || ev.Token != "evm_env" || ev.Base != "https://env.example" || a.EduVaultSource() != "env" {
		t.Fatalf("env: %+v %v %s", ev, err, a.EduVaultSource())
	}
	s := a.Settings()
	s.EduVault = EduVaultSettings{Token: "evm_mine", Secret: "MYSECRET"}
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("settings file %v %v", st, err)
	}
	b := FromEnv() // a restart reads the file
	ev, _ = b.EduVault()
	if ev.Token != "evm_mine" || ev.Secret != "MYSECRET" || ev.Base != "https://eduvault4.de" || b.EduVaultSource() != "settings" {
		t.Fatalf("settings: %+v %s", ev, b.EduVaultSource())
	}
	s.EduVault = EduVaultSettings{}
	b.SaveSettings(s)
	if ev, _ := b.EduVault(); ev.Token != "evm_env" {
		t.Fatalf("cleared settings should fall back to env: %+v", ev)
	}
}

func TestNoEduVault(t *testing.T) {
	t.Setenv("NAK_DATA_DIR", t.TempDir())
	t.Setenv("EDUVAULT_TOKEN", "")
	t.Setenv("EDUVAULT_MCP_SECRET", "")
	if _, err := FromEnv().EduVault(); err == nil {
		t.Fatal("expected not configured")
	}
}

func TestAccountFileWhenEnvIsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NAK_DATA_DIR", dir)
	t.Setenv("CIS_USER", "")
	t.Setenv("CIS_PASS", "")
	t.Setenv("MOODLE_USER", "")
	t.Setenv("MOODLE_PASS", "")
	a := FromEnv()
	if a.AccountUser() != "" || a.AccountSource() != "" {
		t.Fatalf("fresh: %q %q", a.AccountUser(), a.AccountSource())
	}
	if err := a.SaveAccount("12345", "pw"); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "account.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("account file %v %v", st, err)
	}
	b := FromEnv()
	if b.AccountUser() != "12345" || b.AccountSource() != "file" || !b.PasswordMatches("12345", "pw") || b.PasswordMatches("12345", "nope") || b.PasswordMatches("other", "pw") {
		t.Fatalf("reload: %q %q", b.AccountUser(), b.AccountSource())
	}
	if _, err := b.Moodle(); err != nil {
		t.Fatalf("moodle should use the stored account: %v", err)
	}
	t.Setenv("CIS_USER", "999")
	t.Setenv("CIS_PASS", "envpw")
	if c := FromEnv(); c.AccountUser() != "999" || c.AccountSource() != "env" {
		t.Fatalf("env wins: %q %q", c.AccountUser(), c.AccountSource())
	}
}
