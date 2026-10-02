package web

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func calendarURL(t *testing.T, h *harness) string {
	t.Helper()
	m := decode(t, h.do(t, "GET", "/api/calendar", "", bearer))
	u, _ := m["path"].(string)
	if !strings.HasPrefix(u, "/calendar/") || !strings.HasSuffix(u, ".ics") {
		t.Fatalf("calendar path %v", m)
	}
	return u
}

func TestCalendarFeed(t *testing.T) {
	h := newHarness(t)
	h.app.ConfigDir = t.TempDir()
	path := calendarURL(t, h)
	res := h.do(t, "GET", path, "", nil) // calendar apps send no cookie
	b, _ := io.ReadAll(res.Body)
	ics := string(b)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/calendar") {
		t.Fatalf("feed: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "END:VCALENDAR\r\n", "VERSION:2.0",
		`SUMMARY:Softwaretechnik\; Teil 2\, Übung`,
		"DTSTART:20261002T070000Z", "DTEND:20261002T101500Z", // 09:00–12:15 Berlin (CEST)
		"LOCATION:A101",
		"SUMMARY:Klausur: Diskrete Mathematik 2",
		"BEGIN:VALARM", "TRIGGER:-P1D",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("feed lacks %q", want)
		}
	}
	for _, line := range strings.Split(ics, "\r\n") {
		if len(line) > 75 {
			t.Errorf("line not folded (%d octets): %q", len(line), line)
		}
	}
	if strings.Count(ics, "BEGIN:VEVENT") != 3 {
		t.Errorf("events: %d", strings.Count(ics, "BEGIN:VEVENT"))
	}
}

func TestCalendarTokenIsSecretAndRotates(t *testing.T) {
	h := newHarness(t)
	h.app.ConfigDir = t.TempDir()
	old := calendarURL(t, h)
	if res := h.do(t, "GET", "/calendar/wrong.ics", "", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong token: %d", res.StatusCode)
	}
	if res := h.do(t, "POST", "/api/calendar/rotate", `{}`, bearer); res.StatusCode != 200 {
		t.Fatalf("rotate: %d", res.StatusCode)
	}
	if res := h.do(t, "GET", old, "", nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("old link still works: %d", res.StatusCode)
	}
	if calendarURL(t, h) == old {
		t.Fatal("token did not change")
	}
}
