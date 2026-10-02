package web

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const maxUploadFile = 50 << 20

// upload takes files for Moodle submissions. They are kept in the data dir
// for a day — the submit tool reads them from there by path.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*maxUploadFile)
	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "multipart/form-data erwartet"})
		return
	}
	base := filepath.Join(s.app.ConfigDir, "uploads")
	cleanUploads(base, s.cfg.Now())
	b := make([]byte, 8)
	rand.Read(b)
	dir := filepath.Join(base, hex.EncodeToString(b))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type saved struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	var out []saved
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if p.FormName() != "file" || p.FileName() == "" {
			continue
		}
		name := cleanName(p.FileName())
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Datei doppelt: " + name})
			return
		}
		n, err := io.Copy(f, io.LimitReader(p, maxUploadFile+1))
		f.Close()
		if err != nil || n > maxUploadFile {
			os.Remove(path)
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": name + " ist größer als 50 MB"})
			return
		}
		out = append(out, saved{name, path, n})
	}
	if len(out) == 0 {
		os.Remove(dir)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "keine Datei"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

// cleanName keeps the visible file name and drops anything path-like.
func cleanName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimLeft(strings.TrimSpace(name), ".")
	if name == "" {
		return "datei"
	}
	if len(name) > 180 {
		ext := filepath.Ext(name)
		name = name[:180-len(ext)] + ext
	}
	return name
}

func cleanUploads(base string, now time.Time) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
			os.RemoveAll(filepath.Join(base, e.Name()))
		}
	}
}
