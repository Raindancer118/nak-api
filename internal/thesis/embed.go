package thesis

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Semantic matching with a local embedding model (Ollama, e.g. bge-m3): finds
// a reviewer for "Selbstlernende Einordnung von Supportanfragen" although no
// subject area contains those words. Optional: without a model the word
// matching alone decides, as before.

type Embedder interface {
	Embed(texts []string) ([][]float64, error)
}

type embedFunc func([]string) ([][]float64, error)

func (f embedFunc) Embed(t []string) ([][]float64, error) { return f(t) }

// SemanticHint marks an area matched by meaning rather than by words.
const SemanticHint = "inhaltlich ähnlich"

// calibrated with bge-m3 against the NORDAKADEMIE reviewer list (10/2026)
const (
	semMargin = 0.05
	semSpan   = 0.08
)

// ── Ollama ─────────────────────────────────────────────────────────────────

type Ollama struct {
	URL, Model string
	http       *http.Client
}

func NewOllama(url, model string) *Ollama {
	return &Ollama{URL: strings.TrimRight(url, "/"), Model: model, http: &http.Client{Timeout: 2 * time.Minute}}
}

func (o *Ollama) Embed(texts []string) ([][]float64, error) {
	var out [][]float64
	for i := 0; i < len(texts); i += 32 { // a CPU model is happier with small batches
		end := min(i+32, len(texts))
		body, _ := json.Marshal(map[string]any{"model": o.Model, "input": texts[i:end]})
		res, err := o.http.Post(o.URL+"/api/embed", "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		var r struct {
			Embeddings [][]float64 `json:"embeddings"`
			Error      string      `json:"error"`
		}
		err = json.NewDecoder(res.Body).Decode(&r)
		res.Body.Close()
		if err != nil || res.StatusCode != http.StatusOK || len(r.Embeddings) != end-i {
			return nil, fmt.Errorf("embedding model: HTTP %d %s", res.StatusCode, r.Error)
		}
		out = append(out, r.Embeddings...)
	}
	return out, nil
}

// ── cache: subject areas are embedded once, kept on disk ──────────────────

type CachedEmbedder struct {
	inner Embedder
	file  string
	mu    sync.Mutex
	vecs  map[string][]float64
}

func NewCachedEmbedder(inner Embedder, file string) *CachedEmbedder {
	c := &CachedEmbedder{inner: inner, file: file, vecs: map[string][]float64{}}
	if b, err := os.ReadFile(file); err == nil {
		json.Unmarshal(b, &c.vecs)
	}
	return c
}

func key(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func (c *CachedEmbedder) Embed(texts []string) ([][]float64, error) {
	c.mu.Lock()
	var missing []string
	seen := map[string]bool{}
	for _, t := range texts {
		if _, ok := c.vecs[key(t)]; !ok && !seen[t] {
			missing = append(missing, t)
			seen[t] = true
		}
	}
	c.mu.Unlock()
	if len(missing) > 0 {
		vs, err := c.inner.Embed(missing)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		for i, t := range missing {
			c.vecs[key(t)] = vs[i]
		}
		c.save()
		c.mu.Unlock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]float64, len(texts))
	for i, t := range texts {
		out[i] = c.vecs[key(t)]
	}
	return out, nil
}

func (c *CachedEmbedder) save() {
	if c.file == "" {
		return
	}
	if b, err := json.Marshal(c.vecs); err == nil && os.WriteFile(c.file+".tmp", b, 0o600) == nil {
		os.Rename(c.file+".tmp", c.file)
	}
}

// ── scoring ────────────────────────────────────────────────────────────────

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		if i >= len(b) {
			break
		}
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

func profile(r Reviewer) string {
	return strings.TrimSpace(r.Department + ": " + strings.Join(r.Areas, "; "))
}

// SuggestSemantic: the word matching plus the meaning of topic and subject
// areas. Similarities are scaled against the other reviewers (models differ in
// their cosine ranges); a reviewer counts by the better of words and meaning.
func SuggestSemantic(topic string, rs []Reviewer, limit int, emb Embedder) []Match {
	all := ScoreSemantic(topic, rs, emb)
	if all == nil {
		return Suggest(topic, rs, limit)
	}
	return Rank(all, limit)
}

// MatchAmongSemantic: one reviewer, scored like SuggestSemantic does.
func MatchAmongSemantic(topic string, r Reviewer, rs []Reviewer, emb Embedder) Match {
	for _, m := range ScoreSemantic(topic, rs, emb) {
		if m.Reviewer.Name == r.Name {
			return m
		}
	}
	return MatchAmong(topic, r, rs)
}

// ScoreSemantic scores every reviewer; nil when the model is unavailable.
func ScoreSemantic(topic string, rs []Reviewer, emb Embedder) []Match {
	if strings.TrimSpace(topic) == "" || emb == nil || len(rs) == 0 {
		return nil
	}
	texts := []string{topic}
	index := map[string]int{}
	add := func(s string) {
		if _, ok := index[s]; !ok && s != "" {
			index[s] = len(texts)
			texts = append(texts, s)
		}
	}
	for _, r := range rs {
		add(profile(r))
		for _, a := range r.Areas {
			add(a)
		}
	}
	vecs, err := emb.Embed(texts)
	if err != nil || len(vecs) != len(texts) {
		return nil
	}
	tv := vecs[0]
	sem := make([]float64, len(rs))
	bestArea := make([]string, len(rs))
	for i, r := range rs {
		best := 0.0
		for _, a := range r.Areas {
			if s := cosine(tv, vecs[index[a]]); s > best {
				best, bestArea[i] = s, a
			}
		}
		sem[i] = 0.6*best + 0.4*cosine(tv, vecs[index[profile(r)]])
	}
	// Cosines of short labels against a sentence are low and model-specific:
	// a topic's typical reviewer (median) is the baseline, only a clear margin
	// above it counts. Relative to the best one instead, someone would always
	// be a 100 % fit, even for "Orchideen auf dem Balkon".
	sorted := append([]float64(nil), sem...)
	sort.Float64s(sorted)
	floor := sorted[len(sorted)/2] + semMargin
	w := corpusWeights(topic, rs)
	out := make([]Match, 0, len(rs))
	for i, r := range rs {
		m := matchWith(topic, r, w)
		semN := math.Max(0, math.Min(1, (sem[i]-floor)/semSpan))
		if s := 0.5*m.score + 0.5*semN; semN >= 0.15 && s > m.score {
			m.score = s
			m.Percent = int(math.Min(99, math.Round(100*math.Pow(s, 0.75))))
			if semN >= 0.5 && bestArea[i] != "" {
				named := false
				for _, x := range m.Matched {
					named = named || x.Area == bestArea[i]
				}
				if !named {
					m.Matched = append(m.Matched, MatchedArea{Area: bestArea[i], Terms: []string{SemanticHint}})
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// Rank drops non-matches, orders by fit (slightly penalizing busy reviewers)
// and keeps the best limit.
func Rank(all []Match, limit int) []Match {
	var out []Match
	for _, m := range all {
		if m.Percent > 0 {
			out = append(out, m)
		}
	}
	rank := func(m Match) float64 {
		load := m.Reviewer.Load
		if load < 0 {
			load = 1
		}
		return m.score - 0.03*float64(load)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) > rank(out[j]) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
