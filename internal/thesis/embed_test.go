package thesis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEmbedder puts texts on a few axes, like a real model would group them
type fakeEmbedder struct{ calls int }

var axes = [][]string{
	{"kubernetes", "cloud", "devops", "container"},
	{"datenbank", "sql", "warehouse", "data engineering"},
	{"selbstlernend", "einordnung", "machine learning", "generative ki", "data science", "sprachmodell"},
	{"marketing", "kampagne", "vertrieb"},
	{"controlling", "kosten", "kennzahl"},
	{"sicherheit", "kryptographie", "verschluessel"},
}

func (f *fakeEmbedder) Embed(texts []string) ([][]float64, error) {
	f.calls++
	out := make([][]float64, len(texts))
	for i, t := range texts {
		v := make([]float64, len(axes)+1)
		v[len(axes)] = 0.2 // every text shares a little
		low := strings.ToLower(t)
		for a, words := range axes {
			for _, w := range words {
				if strings.Contains(low, w) {
					v[a] += 1
				}
			}
		}
		out[i] = v
	}
	return out, nil
}

func TestSemanticFindsWhatWordsMiss(t *testing.T) {
	topic := "Selbstlernende Einordnung eingehender Supportanfragen"
	if rs := Suggest(topic, people, 3); len(rs) > 0 && top(rs) == "Kurz, Lea" {
		t.Fatal("precondition: words alone should not find this")
	}
	rs := SuggestSemantic(topic, people, 3, &fakeEmbedder{})
	if len(rs) == 0 || top(rs) != "Kurz, Lea" || rs[0].Percent < 30 {
		t.Fatalf("semantic top %+v", rs)
	}
	// it says why: a semantic match is named as such
	found := false
	for _, m := range rs[0].Matched {
		for _, x := range m.Terms {
			if x == SemanticHint {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("matched %+v", rs[0].Matched)
	}
}

func TestSemanticKeepsLexicalWinners(t *testing.T) {
	// a topic the words already solve stays solved
	rs := SuggestSemantic("Performanceoptimierung relationaler Datenbanken", people, 3, &fakeEmbedder{})
	if top(rs) != "Brandt, Jonas" {
		t.Errorf("top %s", top(rs))
	}
}

func TestSemanticFallsBackWithoutModel(t *testing.T) {
	broken := embedFunc(func([]string) ([][]float64, error) { return nil, errors.New("offline") })
	a := SuggestSemantic("Performanceoptimierung relationaler Datenbanken", people, 3, broken)
	b := Suggest("Performanceoptimierung relationaler Datenbanken", people, 3)
	if top(a) != top(b) || a[0].Percent != b[0].Percent {
		t.Errorf("without a model the result must equal the word matching: %v vs %v", a[0].Percent, b[0].Percent)
	}
}

func TestCacheEmbedsEachAreaOnce(t *testing.T) {
	f := &fakeEmbedder{}
	c := NewCachedEmbedder(f, filepath.Join(t.TempDir(), "embed.json"))
	SuggestSemantic("Cloud", people, 3, c)
	n := f.calls
	SuggestSemantic("Datenbank", people, 3, c)
	if f.calls != n+1 { // only the new topic
		t.Errorf("calls %d → %d: areas must come from the cache", n, f.calls)
	}
	// a new cache from the file needs no area embedding at all
	f2 := &fakeEmbedder{}
	c2 := NewCachedEmbedder(f2, c.file)
	SuggestSemantic("Cloud", people, 3, c2)
	if f2.calls != 0 {
		t.Errorf("persisted cache unused: %d calls", f2.calls)
	}
}

func TestOllamaEmbedder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if r.URL.Path != "/api/embed" || in.Model != "bge-m3" {
			http.Error(w, "bad", 400)
			return
		}
		out := map[string]any{"embeddings": [][]float64{}}
		for range in.Input {
			out["embeddings"] = append(out["embeddings"].([][]float64), []float64{1, 0})
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	e := NewOllama(srv.URL, "bge-m3")
	v, err := e.Embed([]string{"a", "b"})
	if err != nil || len(v) != 2 || v[1][0] != 1 {
		t.Fatalf("%v %v", v, err)
	}
}
