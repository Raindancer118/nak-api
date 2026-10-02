package tools

import (
	"errors"
	"testing"

	"github.com/Raindancer118/nak-api/internal/thesis"
)

type fakeEmb func([]string) ([][]float64, error)

func (f fakeEmb) Embed(t []string) ([][]float64, error) { return f(t) }

func TestMatchResultReportsSemantic(t *testing.T) {
	rs := []thesis.Reviewer{{Name: "A", Areas: []string{"Datenbanken"}}, {Name: "B", Areas: []string{"Marketing"}}}
	vec := fakeEmb(func(ts []string) ([][]float64, error) {
		out := make([][]float64, len(ts))
		for i := range ts {
			out[i] = []float64{1, float64(i)}
		}
		return out, nil
	})
	r, err := matchResult("Datenbanken", "", rs, vec)
	if err != nil || r.(map[string]any)["semantic"] != true {
		t.Fatalf("semantic flag missing: %v %v", r, err)
	}
	broken := fakeEmb(func([]string) ([][]float64, error) { return nil, errors.New("down") })
	r, _ = matchResult("Datenbanken", "", rs, broken)
	if r.(map[string]any)["semantic"] != false {
		t.Fatalf("a failing model must fall back to word matching: %v", r)
	}
	r, _ = matchResult("Datenbanken", "A", rs, nil)
	if r.(map[string]any)["semantic"] != false || r.(map[string]any)["match"].(thesis.Match).Percent == 0 {
		t.Fatalf("without a model: %v", r)
	}
}

func TestEmbedderFromEnv(t *testing.T) {
	t.Setenv("NAK_EMBED_URL", "")
	if e := embedderFor(t.TempDir()); e != nil {
		t.Fatal("no URL, no embedder")
	}
	t.Setenv("NAK_EMBED_URL", "http://ollama:11434")
	if e := embedderFor(t.TempDir()); e == nil {
		t.Fatal("URL set, embedder expected")
	}
}
