package web

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryKeepsPastExamsAndGradeSteps(t *testing.T) {
	file := filepath.Join(t.TempDir(), "history.json")
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	hs := newHistory(file, func() time.Time { return now })
	exams := func(list ...map[string]any) json.RawMessage { b, _ := json.Marshal(list); return b }
	hs.observe("cis_list_klausuren", exams(
		map[string]any{"exam_id": "1", "module_nr": "A222,I222", "title": "Diskrete Mathematik 2", "start": "02.10.2026 11:30", "ende": "02.10.2026 13:00", "registered": true, "dozenten": []string{"Brzezinski"}},
		map[string]any{"exam_id": "2", "module_nr": "I160", "title": "Datenbanken", "start": "20.10.2026 09:00", "registered": false},
	))
	// after the exam the CIS no longer lists it
	now = now.Add(48 * time.Hour)
	hs.observe("cis_list_klausuren", exams(map[string]any{"exam_id": "2", "module_nr": "I160", "title": "Datenbanken", "start": "20.10.2026 09:00", "registered": true}))
	grades := func(grade, date string, attempt int) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"overview": map[string]any{"modules": []map[string]any{{"module_nr": "I168", "title": "Diskrete Mathematik 2", "grade": grade, "status": "x", "exam_date": date, "attempt": attempt}}}})
		return b
	}
	hs.observe("cis_grades", grades("5,0 (2.Versuch)", "11.02.2026", 2))
	hs.observe("cis_grades", grades("5,0 (2.Versuch)", "11.02.2026", 2)) // unchanged
	hs.observe("cis_grades", grades("2,7 (3.Versuch)", "02.10.2026", 3))

	again := newHistory(file, time.Now) // persisted
	s := again.snapshot()
	if len(s.Exams) != 2 {
		t.Fatalf("exams = %d", len(s.Exams))
	}
	var dm2, db examRecord
	for _, e := range s.Exams {
		switch e.ExamID {
		case "1":
			dm2 = e
		case "2":
			db = e
		}
	}
	if !dm2.Registered || dm2.Start != "02.10.2026 11:30" || len(dm2.Dozenten) != 1 {
		t.Fatalf("past exam lost details: %+v", dm2)
	}
	if !db.Registered {
		t.Fatalf("registration not updated: %+v", db)
	}
	if g := s.Grades["I168"]; len(g) != 2 || g[1].Attempt != 3 || g[0].ExamDate != "11.02.2026" {
		t.Fatalf("grade steps: %+v", g)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	h := newHarness(t)
	h.web.history.observe("cis_list_klausuren", json.RawMessage(`[{"exam_id":"9","module_nr":"I151","title":"SWT","start":"01.09.2026 08:00","registered":true}]`))
	m := decode(t, h.do(t, "GET", "/api/history", "", bearer))
	if ex, _ := m["exams"].([]any); len(ex) != 1 {
		t.Fatalf("history: %v", m)
	}
	if res := h.do(t, "GET", "/api/history", "", nil); res.StatusCode != 401 {
		t.Fatalf("no auth: %d", res.StatusCode)
	}
}
