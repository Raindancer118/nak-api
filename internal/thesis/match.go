package thesis

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Topic ↔ reviewer matching, done here and not by an external AI: the topic
// stays on the owner's server and the result can be explained. Two signals:
// the topic's words against the reviewer's subject areas (stems, German
// compounds: "Datenbankperformance" meets "Datenbanksysteme"), and concepts
// from a dictionary of the NORDAKADEMIE's fields, so "Kubernetes" finds
// "Cloud Computing" and "Large Language Models" finds "Generative KI".

type concept struct {
	name  string
	words []string // folded; three letters or fewer must be a whole word
}

var concepts = []concept{
	{"Datenbanken", []string{"datenbank", "sql", "nosql", "relational", "datenmodell", "abfrage", "query", "olap", "data warehouse", "datawarehouse", "data engineering", "etl", "indexierung", "stammdaten", "masterdata", "master data", "mdm", "datenqualitat", "data quality", "data governance", "datenmanagement", "data management", "datenintegration"}},
	{"KI & Data Science", []string{"kunstliche intelligenz", "ki", "ai", "machine learning", "maschinelles lernen", "ml", "deep learning", "neuronal", "neural", "llm", "language model", "sprachmodell", "generativ", "chatbot", "data science", "data analytics", "datenanalyse", "prognose", "vorhersage", "klassifikation", "klassifizier", "nlp", "computer vision"}},
	{"Cloud & DevOps", []string{"cloud", "aws", "azure", "gcp", "kubernetes", "container", "docker", "devops", "ci", "cd", "pipeline", "microservice", "serverless", "infrastructure as code", "terraform", "verteilte system"}},
	{"IT-Sicherheit", []string{"sicherheit", "security", "penetration", "pentest", "angriff", "verschlussel", "krypto", "siem", "soc", "iso 27001", "bsi", "informationssicherheit", "schwachstell", "phishing", "zugriffsschutz", "authentifizier"}},
	{"Netzwerke", []string{"netzwerk", "rechnernetz", "protokoll", "tcp", "routing", "wlan", "5g", "iot", "internet of things"}},
	{"Softwaretechnik", []string{"softwaretechnik", "softwareentwicklung", "softwarearchitektur", "architektur", "refactoring", "clean code", "testautomatisier", "softwaretest", "qualitatssicherung software", "anforderung", "requirements", "uml", "agil", "scrum", "kanban", "programmier", "programmiersprach", "code"}},
	{"Web & E-Commerce", []string{"web", "frontend", "backend", "javascript", "react", "rest", "api", "e-commerce", "ecommerce", "onlineshop", "online-shop", "webshop", "internet anwendung"}},
	{"Betriebliche Anwendungssysteme", []string{"erp", "sap", "crm", "betriebliche anwendung", "anwendungssystem", "geschaftsprozess", "prozessmanagement", "bpmn", "workflow", "rpa", "prozessautomatisier", "digitalisier"}},
	{"Analytische Informationssysteme", []string{"business intelligence", "bi", "dashboard", "reporting", "analytische informationssystem", "power bi", "tableau"}},
	{"Marketing & Vertrieb", []string{"marketing", "marke", "brand", "vertrieb", "sales", "kampagne", "social media", "social-media", "seo", "b2b", "b2c", "kundenbind", "kundenzufrieden", "customer", "werbung", "influencer"}},
	{"Controlling & Finanzen", []string{"controlling", "kennzahl", "kpi", "kosten", "budget", "finanzierung", "investition", "bilanz", "rechnungswesen", "buchhaltung", "steuer", "corporate finance", "leistungsrechnung", "rentabilitat"}},
	{"Logistik & Supply Chain", []string{"logistik", "supply chain", "lieferkette", "lager", "transport", "beschaffung", "einkauf", "operations management", "intralogistik"}},
	{"Personal & HR", []string{"personal", "hr", "mitarbeiter", "mitarbeitend", "beschaftigte", "arbeitgeber", "employer branding", "recruiting", "fachkraft", "talent", "generation z", "fluktuation", "onboarding", "weiterbildung", "personalentwicklung", "arbeitswelt", "new work", "remote work", "homeoffice", "arbeitspsycholog"}},
	{"Management & Organisation", []string{"projektmanagement", "projekt", "change management", "veranderung", "organisation", "fuhrung", "leadership", "strategie", "strategisch", "innovation", "unternehmensmodellier", "geschaftsmodell"}},
	{"Mathematik & Statistik", []string{"statistik", "mathematik", "optimierung", "simulation", "spieltheorie", "stochastik", "regression", "operations research"}},
	{"Ingenieurwesen", []string{"maschinenbau", "produktion", "fertigung", "automatisierungstechnik", "elektrotechnik", "mechatronik", "qualitatsmanagement", "konstruktion", "werkstoff", "energie", "nachhaltigkeit", "umwelt", "thermodynamik"}},
	{"Recht", []string{"recht", "dsgvo", "datenschutz", "vertrag", "arbeitsrecht", "compliance"}},
}

var (
	nonWord   = regexp.MustCompile(`[^a-z0-9]+`)
	stopwords = map[string]bool{}
	suffixes  = []string{"ungen", "ung", "ern", "en", "er", "es", "e", "s", "n"}
	keepShort = map[string]bool{"ki": true, "ai": true, "ml": true, "it": true, "bi": true, "hr": true, "5g": true}
)

func init() {
	for _, w := range strings.Fields("der die das den dem des und oder fur von vom im in mit zur zum bei eines einer einem ein eine auf durch als am an aus bzw etc sowie uber unter nach vor bis zu wie was welche welcher welches eines anhand mittels hinsichtlich bezug am beispiel fallstudie unternehmen betrieb entwicklung konzept konzeption analyse einsatz einfuhrung untersuchung gestaltung bewertung optimierung ansatz ansatze moglichkeiten") {
		stopwords[w] = true
	}
}

func fold(s string) string {
	return strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "ß", "ss", "Ä", "a", "Ö", "o", "Ü", "u").Replace(strings.ToLower(s))
}

func stem(w string) string {
	for _, suf := range suffixes {
		if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 4 {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

// terms: the meaningful words of a text, stemmed
func terms(s string) []string {
	out, _ := termsWithWords(s)
	return out
}

// termsWithWords also returns the word as written for each stem, to show
// "Kubernetes" rather than "kubernet"
func termsWithWords(s string) ([]string, map[string]string) {
	words := map[string]string{}
	var out []string
	for _, raw := range strings.FieldsFunc(s, func(r rune) bool { return !(r == '-' || r == '/' || unicode.IsLetter(r) || unicode.IsDigit(r)) }) {
		for _, w := range strings.Fields(nonWord.ReplaceAllString(fold(raw), " ")) {
			if stopwords[w] || (len(w) < 3 && !keepShort[w]) {
				continue
			}
			st := stem(w)
			if _, ok := words[st]; !ok {
				words[st] = strings.Trim(raw, "-/")
				out = append(out, st)
			}
		}
	}
	return out, words
}

func hasConcept(text string, c concept) bool {
	t := " " + nonWord.ReplaceAllString(fold(text), " ") + " "
	for _, w := range c.words {
		if len(w) <= 3 {
			if strings.Contains(t, " "+w+" ") {
				return true
			}
		} else if strings.Contains(t, w) || strings.Contains(strings.ReplaceAll(t, " ", ""), strings.ReplaceAll(w, " ", "")) {
			return true
		}
	}
	return false
}

// genericPrefix: shared word starts that say nothing about the subject
// ("Masterdata" is no "Masterstudiengang")
func genericPrefix(p string) bool {
	for _, g := range []string{"master", "bachelor", "studien", "grundlag", "allgemein", "angewandt"} {
		if strings.HasPrefix(g, p) || strings.HasPrefix(p, g) {
			return true
		}
	}
	return false
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func trigrams(s string) map[string]bool {
	s = "  " + s + " "
	out := map[string]bool{}
	for i := 0; i+3 <= len(s); i++ {
		out[s[i:i+3]] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	in := 0
	for k := range a {
		if b[k] {
			in++
		}
	}
	if u := len(a) + len(b) - in; u > 0 {
		return float64(in) / float64(u)
	}
	return 0
}

// termFit: how well one topic word is covered by an area's words
func termFit(t string, area []string) float64 {
	best := 0.0
	for _, a := range area {
		switch {
		case a == t:
			return 1
		case len(t) >= 5 && len(a) >= 5 && (strings.Contains(a, t) || strings.Contains(t, a)):
			best = math.Max(best, 0.85)
		case commonPrefix(a, t) >= 6 && !genericPrefix(a[:commonPrefix(a, t)]):
			best = math.Max(best, 0.8)
		default:
			if j := jaccard(trigrams(t), trigrams(a)); j >= 0.45 {
				best = math.Max(best, 0.8*j)
			}
		}
	}
	return best
}

type MatchedArea struct {
	Area  string   `json:"area"`
	Terms []string `json:"terms"` // topic words or concepts it covers
}

type Match struct {
	Reviewer  Reviewer      `json:"reviewer"`
	Percent   int           `json:"percent"`
	Matched   []MatchedArea `json:"matched"`
	Uncovered []string      `json:"uncovered"` // parts of the topic this reviewer does not cover
	score     float64
}

const conceptWeight = 1.5

// weights: how much each topic word and concept counts. Against a list of
// reviewers, words many of them share ("data") count less than rare ones.
type weights struct {
	term    map[string]float64
	concept map[string]float64
}

func corpusWeights(topic string, rs []Reviewer) weights {
	w := weights{term: map[string]float64{}, concept: map[string]float64{}}
	n := float64(len(rs))
	if n == 0 {
		return w
	}
	idf := func(df int) float64 { return 0.3 + 0.7*math.Log((n+1)/(float64(df)+1))/math.Log(n+1) }
	areaTerms := make([][][]string, len(rs))
	for i, r := range rs {
		for _, a := range r.Areas {
			areaTerms[i] = append(areaTerms[i], terms(a))
		}
	}
	for _, t := range terms(topic) {
		df := 0
		for i := range rs {
			for _, at := range areaTerms[i] {
				if termFit(t, at) >= 0.8 {
					df++
					break
				}
			}
		}
		w.term[t] = idf(df)
	}
	for _, c := range concepts {
		if !hasConcept(topic, c) {
			continue
		}
		df := 0
		for _, r := range rs {
			for _, a := range r.Areas {
				if hasConcept(a, c) {
					df++
					break
				}
			}
		}
		w.concept[c.name] = idf(df)
	}
	return w
}

// MatchOne scores one reviewer for a topic (0–99 %) on its own.
func MatchOne(topic string, r Reviewer) Match { return matchWith(topic, r, weights{}) }

// MatchAmong scores one reviewer with the weights of the whole list, as
// Suggest does, so both show the same percentage.
func MatchAmong(topic string, r Reviewer, all []Reviewer) Match {
	return matchWith(topic, r, corpusWeights(topic, all))
}

func matchWith(topic string, r Reviewer, w weights) Match {
	m := Match{Reviewer: r, Matched: []MatchedArea{}, Uncovered: []string{}}
	ts, written := termsWithWords(topic)
	var topicConcepts []concept
	for _, c := range concepts {
		if hasConcept(topic, c) {
			topicConcepts = append(topicConcepts, c)
		}
	}
	tw := func(t string) float64 {
		if x, ok := w.term[t]; ok {
			return x
		}
		return 1
	}
	cw := func(c string) float64 {
		if x, ok := w.concept[c]; ok {
			return conceptWeight * x
		}
		return conceptWeight
	}
	possible := 0.0
	for _, t := range ts {
		possible += tw(t)
	}
	for _, c := range topicConcepts {
		possible += cw(c.name)
	}
	if possible == 0 {
		return m
	}
	areaTerms := make([][]string, len(r.Areas))
	for i, a := range r.Areas {
		areaTerms[i] = terms(a)
	}
	byArea := map[string][]string{}
	add := func(area, term string) {
		for _, x := range byArea[area] {
			if x == term {
				return
			}
		}
		byArea[area] = append(byArea[area], term)
	}
	got := 0.0
	for _, t := range ts {
		best, where := 0.0, ""
		for i, at := range areaTerms {
			if f := termFit(t, at); f > best {
				best, where = f, r.Areas[i]
			}
		}
		if best >= 0.3 {
			got += best * tw(t)
			add(where, written[t])
		} else {
			m.Uncovered = append(m.Uncovered, written[t])
		}
	}
	for _, c := range topicConcepts {
		where := ""
		for _, a := range r.Areas {
			if hasConcept(a, c) {
				where = a
				break
			}
		}
		if where != "" {
			got += cw(c.name)
			add(where, c.name)
		}
	}
	m.score = got / possible
	m.Percent = int(math.Min(99, math.Round(100*math.Pow(m.score, 0.75))))
	for _, a := range r.Areas {
		if ts := byArea[a]; len(ts) > 0 {
			m.Matched = append(m.Matched, MatchedArea{Area: a, Terms: ts})
		}
	}
	return m
}

// Suggest ranks the reviewers for a topic: fit first, then free capacity.
func Suggest(topic string, rs []Reviewer, limit int) []Match {
	var out []Match
	w := corpusWeights(topic, rs)
	for _, r := range rs {
		if m := matchWith(topic, r, w); m.Percent > 0 {
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
