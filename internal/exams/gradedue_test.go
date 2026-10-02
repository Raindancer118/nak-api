package exams

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.UTC)
	return t
}

func TestGradeDueCountsLectureWeeksOnly(t *testing.T) {
	exam := day("2026-10-02") // Friday
	lectures := []time.Time{
		day("2026-10-01"),                    // before the exam: does not count
		day("2026-10-05"), day("2026-10-07"), // week 1
		day("2026-10-12"), // week 2
		// 19.10.–08.11.: Praxisphase, no lectures
		day("2026-11-09"), // week 3
		day("2026-11-17"), // week 4
		day("2026-11-23"), // later
	}
	due, ok := GradeDue(exam, lectures, false)
	if !ok || !due.Equal(day("2026-11-22")) {
		t.Fatalf("due %v ok %v, want end of the 4th lecture week (Sun 22.11.2026)", due, ok)
	}
}

func TestGradeDueUnknownWhenThePlanEndsTooEarly(t *testing.T) {
	_, ok := GradeDue(day("2026-10-02"), []time.Time{day("2026-10-05"), day("2026-10-13")}, false)
	if ok {
		t.Fatal("two known lecture weeks cannot give a deadline of four")
	}
}

func TestGradeDueMasterIsSixCalendarWeeks(t *testing.T) {
	due, ok := GradeDue(day("2026-10-02"), nil, true)
	if !ok || !due.Equal(day("2026-11-13")) {
		t.Fatalf("master due %v", due)
	}
}
