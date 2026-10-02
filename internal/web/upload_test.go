package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func multipartBody(t *testing.T, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, content := range files {
		w, _ := mw.CreateFormFile("file", name)
		w.Write([]byte(content))
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestUploadStoresFilesPrivately(t *testing.T) {
	h := newHarness(t)
	h.app.ConfigDir = t.TempDir()
	body, ctype := multipartBody(t, map[string]string{"../../etc/Abgabe 1.pdf": "%PDF-1.7 meine Abgabe"})
	req, _ := http.NewRequest("POST", h.srv.URL+"/api/upload", body)
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Files []struct {
			Name, Path string
			Size       int64
		}
	}
	json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 200 || len(out.Files) != 1 {
		t.Fatalf("upload: %d %+v", res.StatusCode, out)
	}
	f := out.Files[0]
	if f.Name != "Abgabe 1.pdf" || !strings.HasPrefix(f.Path, filepath.Join(h.app.ConfigDir, "uploads")) {
		t.Fatalf("stored as %+v", f)
	}
	if b, _ := os.ReadFile(f.Path); string(b) != "%PDF-1.7 meine Abgabe" {
		t.Fatalf("content %q", b)
	}
	if st, _ := os.Stat(f.Path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
}

func TestUploadNeedsAuthAndSameOrigin(t *testing.T) {
	h := newHarness(t)
	h.app.ConfigDir = t.TempDir()
	for _, hdr := range []map[string]string{nil, with(map[string]string{"Sec-Fetch-Site": "cross-site"})} {
		body, ctype := multipartBody(t, map[string]string{"a.pdf": "x"})
		req, _ := http.NewRequest("POST", h.srv.URL+"/api/upload", body)
		req.Header.Set("Content-Type", ctype)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, _ := http.DefaultClient.Do(req)
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatalf("upload accepted with headers %v", hdr)
		}
	}
}

func TestOldUploadsAreRemoved(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "uploads", "old")
	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "x.pdf"), []byte("x"), 0o600)
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(old, past, past)
	cleanUploads(filepath.Join(dir, "uploads"), time.Now())
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old upload kept")
	}
}
