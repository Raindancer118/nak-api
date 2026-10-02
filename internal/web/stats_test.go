package web

import (
	"io"
	"strings"
	"testing"
)

func TestStatsCountFetchesAndCacheHits(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 3; i++ {
		h.do(t, "POST", "/api/tools/fake_read", `{"q":"x"}`, bearer)
	}
	m := decode(t, h.do(t, "GET", "/api/stats", "", bearer))
	if m["fetches"] != float64(1) || m["cache_hits"] != float64(2) {
		t.Fatalf("stats: %v", m)
	}
	per, _ := m["tools"].(map[string]any)
	if fr, _ := per["fake_read"].(map[string]any); fr["fetches"] != float64(1) || fr["hits"] != float64(2) {
		t.Fatalf("per tool: %v", per)
	}
}

func TestMetricsNeedAuthAndArePrometheusText(t *testing.T) {
	h := newHarness(t)
	h.do(t, "POST", "/api/tools/fake_read", `{}`, bearer)
	if res := h.do(t, "GET", "/metrics", "", nil); res.StatusCode != 401 {
		t.Fatalf("metrics without auth: %d", res.StatusCode)
	}
	res := h.do(t, "GET", "/metrics", "", bearer)
	b, _ := io.ReadAll(res.Body)
	body := string(b)
	for _, want := range []string{"# TYPE naknak_upstream_fetches_total counter", `naknak_upstream_fetches_total{tool="fake_read"} 1`, "naknak_cache_hits_total", "naknak_up 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics lack %q\n%s", want, body)
		}
	}
}
