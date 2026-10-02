package moodle

import (
	"testing"
	"time"
)

func TestIdenticalReadsAreShared(t *testing.T) {
	f := newFake(t).on("core_course_get_contents", `[]`).on("core_message_send_instant_messages", `[{"msgid":1}]`)
	c := f.client()
	now := time.Unix(1000, 0)
	c.CacheTTL, c.Now = time.Minute, func() time.Time { return now }
	count := func() int {
		n := 0
		for _, fn := range f.called() {
			if fn == "core_course_get_contents" {
				n++
			}
		}
		return n
	}
	c.Call("core_course_get_contents", map[string]any{"courseid": 2})
	c.Call("core_course_get_contents", map[string]any{"courseid": 2})
	c.Call("core_course_get_contents", map[string]any{"courseid": 3})
	if count() != 2 {
		t.Fatalf("calls = %d, want 2", count())
	}
	now = now.Add(2 * time.Minute)
	c.Call("core_course_get_contents", map[string]any{"courseid": 2})
	if count() != 3 {
		t.Fatalf("expired entry reused: %d", count())
	}
	c.CallWrite("core_message_send_instant_messages", map[string]any{"messages": []any{}})
	c.Call("core_course_get_contents", map[string]any{"courseid": 2})
	if count() != 4 {
		t.Fatalf("cache survived a write: %d", count())
	}
}

func TestMoodleErrorsAreNotCached(t *testing.T) {
	f := newFake(t)
	c := f.client()
	c.CacheTTL = time.Minute
	c.Call("missing_fn", nil)
	c.Call("missing_fn", nil)
	n := 0
	for _, fn := range f.called() {
		if fn == "missing_fn" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("error cached: %d calls", n)
	}
}
