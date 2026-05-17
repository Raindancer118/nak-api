package certs

import (
	"fmt"
	"io"
	"strings"

	"github.com/Raindancer118/cis-api/internal/client"
	"github.com/Raindancer118/cis-api/internal/scraper"
)

type Certificate struct {
	Name        string
	DownloadURL string
}

// FetchList fetches the online-bescheinigungen page and returns downloadable
// certificates. Download URLs are extracted directly from anchor hrefs.
func FetchList(c *client.Client) ([]Certificate, error) {
	resp, err := c.Get("/mein-profil/mein-postfach/online-bescheinigungen")
	if err != nil {
		return nil, fmt.Errorf("fetch certs page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// dumpFile links contain the actual downloadable PDFs
	links, err := scraper.ExtractLinks(strings.NewReader(string(body)), "eID=dumpFile")
	if err != nil {
		return nil, fmt.Errorf("parse cert links: %w", err)
	}

	seen := map[string]bool{}
	var certs []Certificate
	for _, l := range links {
		if seen[l] {
			continue
		}
		seen[l] = true
		name := extractCertName(l)
		dlURL := l
		if !strings.HasPrefix(dlURL, "http") {
			dlURL = client.BaseURL + dlURL
		}
		certs = append(certs, Certificate{Name: name, DownloadURL: dlURL})
	}

	// Fallback: look for any anchors whose text looks like certificates
	if len(certs) == 0 {
		certs = extractByLinkText(string(body))
	}
	return certs, nil
}

// Download fetches a certificate and returns its raw bytes.
func Download(c *client.Client, downloadURL string) ([]byte, string, error) {
	resp, err := c.GetURL(downloadURL)
	if err != nil {
		return nil, "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, contentType, fmt.Errorf("read download: %w", err)
	}
	return data, contentType, nil
}

func extractCertName(href string) string {
	// Try to extract a human-readable name from the URL parameters
	parts := strings.Split(href, "&")
	for _, p := range parts {
		if strings.HasPrefix(p, "f=") || strings.HasPrefix(p, "file=") {
			return strings.TrimPrefix(strings.TrimPrefix(p, "f="), "file=")
		}
	}
	return "Bescheinigung"
}

func extractByLinkText(body string) []Certificate {
	// Minimal extraction from raw HTML when structured links aren't found
	var certs []Certificate
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "bescheinigung") || strings.Contains(lower, "immatrikulation") {
			if strings.Contains(line, "href=") {
				href := extractHref(line)
				if href != "" && !strings.Contains(href, "#") {
					if !strings.HasPrefix(href, "http") {
						href = client.BaseURL + href
					}
					certs = append(certs, Certificate{Name: "Bescheinigung", DownloadURL: href})
				}
			}
		}
	}
	return certs
}

func extractHref(line string) string {
	start := strings.Index(line, `href="`)
	if start == -1 {
		return ""
	}
	start += 6
	end := strings.Index(line[start:], `"`)
	if end == -1 {
		return ""
	}
	return line[start : start+end]
}
