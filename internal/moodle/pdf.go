package moodle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// PDFText extracts text page by page ("--- Seite n ---"). Poppler's
// pdftotext gives the best layout and is used when installed; otherwise a
// pure-Go extractor runs.
func PDFText(data []byte) (string, error) {
	var pages []string
	var err error
	if path, lookErr := exec.LookPath("pdftotext"); lookErr == nil {
		pages, err = popplerPages(path, data)
	}
	if pages == nil {
		pages, err = goPages(data)
	}
	if err != nil {
		return "", &Error{Code: "pdf", Msg: "could not parse PDF: " + err.Error()}
	}
	var sb strings.Builder
	hasText := false
	for i, p := range pages {
		p = strings.TrimSpace(p)
		hasText = hasText || p != ""
		fmt.Fprintf(&sb, "--- Seite %d ---\n%s\n\n", i+1, p)
	}
	if !hasText {
		return "", &Error{Code: "no_text", Msg: "PDF has no extractable text (scanned?) – use moodle_download instead"}
	}
	return strings.TrimSpace(sb.String()), nil
}

func popplerPages(bin string, data []byte) ([]string, error) {
	f, err := os.CreateTemp("", "nak-pdf-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-layout", "-enc", "UTF-8", f.Name(), "-").Output()
	if err != nil {
		return nil, nil // fall back to the Go extractor
	}
	pages := strings.Split(string(out), "\f")
	if len(pages) > 1 && strings.TrimSpace(pages[len(pages)-1]) == "" {
		pages = pages[:len(pages)-1]
	}
	return pages, nil
}

func goPages(data []byte) (pages []string, err error) {
	defer func() {
		if r := recover(); r != nil { // the library panics on some malformed files
			err = fmt.Errorf("%v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			pages = append(pages, "")
			continue
		}
		t, err := p.GetPlainText(nil)
		if err != nil {
			return nil, err
		}
		pages = append(pages, t)
	}
	return pages, nil
}
