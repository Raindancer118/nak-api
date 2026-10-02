package timetable

import (
	"os"
	"testing"
	"time"
)

func load(t *testing.T) []Event {
	t.Helper()
	b, err := os.ReadFile("testdata/zenturie.ics")
	if err != nil {
		t.Fatal(err)
	}
	evs, err := ParseICS(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestParseICS(t *testing.T) {
	evs := load(t)
	if len(evs) != 8 {
		t.Fatalf("events = %d", len(evs))
	}
	var lecture *Event
	for i := range evs {
		if evs[i].ModuleNr == "I151" {
			lecture = &evs[i]
			break
		}
	}
	if lecture == nil {
		t.Fatal("no I151 event")
	}
	if lecture.Kind != "V" || lecture.KindName != "Vorlesung" || lecture.Title != "Softwaretechnik" ||
		lecture.Lecturer != "Prof. Dr. rer. nat. Sehring" || lecture.Room != "A006" {
		t.Errorf("lecture = %+v", lecture)
	}
	if lecture.Start.Location().String() != "Europe/Berlin" || lecture.Start.Hour() != 9 || lecture.Start.Minute() != 15 {
		t.Errorf("start = %v", lecture.Start)
	}
	if lecture.Pause != "" || lecture.Note != "" {
		t.Errorf("'-' must become empty: %+v", lecture)
	}
	for _, e := range evs {
		if e.Kind == "WP" {
			if e.Title == "" || e.Pause != "inkl. 30 min Pause" || e.ModuleNr != "" {
				t.Errorf("WP event = %+v", e)
			}
		}
	}
}

func TestFilterAndSort(t *testing.T) {
	evs := load(t)
	from := time.Date(2026, 7, 27, 0, 0, 0, 0, Berlin)
	to := from.AddDate(0, 0, 1)
	got := Filter(evs, Query{From: from, To: to})
	if len(got) == 0 {
		t.Fatal("no events on 27.07.")
	}
	for i := 1; i < len(got); i++ {
		if got[i].Start.Before(got[i-1].Start) {
			t.Fatal("not sorted")
		}
	}
	if n := len(Filter(evs, Query{Kind: "WP"})); n != 3 {
		t.Errorf("WP events = %d", n)
	}
	if n := len(Filter(evs, Query{Text: "softwaretechnik"})); n != 2 {
		t.Errorf("text filter = %d", n)
	}
	if n := len(Filter(evs, Query{ExcludeWPExcept: []string{"Digital Commerce"}})); n != 6 {
		t.Errorf("WP restriction = %d", n)
	}
}

func TestUnfoldAndUnescape(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nSUMMARY:a\\, b\r\nDESCRIPTION:Veranstaltung: V I100 Te\r\n st\\nDozent: Dr. X\\nRaum: R1\r\nDTSTART:20260101T080000Z\r\nDTEND:20260101T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	evs, err := ParseICS(ics)
	if err != nil || len(evs) != 1 {
		t.Fatalf("%v %d", err, len(evs))
	}
	e := evs[0]
	if e.Title != "Test" || e.ModuleNr != "I100" || e.Lecturer != "Dr. X" || e.Start.UTC().Hour() != 8 {
		t.Errorf("event = %+v", e)
	}
}
