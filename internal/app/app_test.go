package app

import (
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
