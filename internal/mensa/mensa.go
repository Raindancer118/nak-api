// Package mensa reads the NORDAKADEMIE canteen (mensa.nordakademie.de, OPC
// WebBestellung): the menu is public, balance and bookings need the NAK login
// (LDAP). OPC wants the cookie consent before it accepts a login.
package mensa

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultURL = "https://mensa.nordakademie.de"

var ErrAuth = errors.New("Mensa-Anmeldung fehlgeschlagen (NAK-Login)")

type Dish struct {
	Date      string   `json:"date"` // YYYY-MM-DD
	Line      string   `json:"line"` // "Menü 1", "Menü 2 Veg."
	Name      string   `json:"name"`
	Price     float64  `json:"price"`
	Tags      []string `json:"tags,omitempty"`
	Allergens []string `json:"allergens,omitempty"`
	Additives []string `json:"additives,omitempty"`
}

func (d Dish) Vegetarian() bool {
	for _, t := range d.Tags {
		if t == "Vegetarisch" || t == "Vegan" {
			return true
		}
	}
	return strings.Contains(strings.ToLower(d.Line), "veg")
}

// symbols of the OPC "Symbolik" bit mask, as the menu legend names them
var symbols = []struct {
	bit  int
	name string
}{{1, "Vegetarisch"}, {2, "Rind"}, {4, "Schwein"}, {8, "Geflügel"}, {16, "Fisch"}, {32, "Lamm"}, {64, "Wild"}, {128, "Laktosefrei"}, {256, "Vegan"}, {512, "Schlanker Tag"}}

var (
	offerRe  = regexp.MustCompile(`OPC\.angebote\.put\(\d+,(\{.*?\})\);`)
	columnRe = regexp.MustCompile(`data-spalte="spalte_(\d+)"\s*>([^<]+)</th>`)
	hashRe   = regexp.MustCompile(`id="WIDHASH(\d+)" value="(\w+)"`)
	brRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
)

func clean(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(s, " "))), " ")
}

// "a: Eier und daraus gewonnene Erzeugnisse<br>…" → ["Eier", …]
func legend(s string) []string {
	var out []string
	for _, p := range brRe.Split(s, -1) {
		p = clean(p)
		if i := strings.Index(p, ": "); i >= 0 && i <= 3 {
			p = p[i+2:]
		}
		p = strings.TrimSpace(strings.TrimSuffix(p, " und daraus gewonnene Erzeugnisse"))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ParseMenu(page string) []Dish {
	cols := map[int]string{}
	for _, m := range columnRe.FindAllStringSubmatch(page, -1) {
		n, _ := strconv.Atoi(m[1])
		cols[n] = clean(m[2])
	}
	var out []Dish
	for _, m := range offerRe.FindAllStringSubmatch(page, -1) {
		var o struct {
			Datum, ArtBez, Bon         string
			Spalte, Symbolik           int
			ArtPreis                   string
			AllergeneFromTableAsString string
			ZusatzstoffeAsString       string
			Geloescht                  int
		}
		if json.Unmarshal([]byte(m[1]), &o) != nil || o.Geloescht != 0 {
			continue
		}
		line := cols[o.Spalte]
		if line == "" {
			line = clean(o.Bon)
		}
		name := clean(o.ArtBez)
		// an empty slot repeats its line name ("Menü 2 veg.")
		if name == "" || strings.EqualFold(name, line) || strings.EqualFold(name, clean(o.Bon)) {
			continue
		}
		price, _ := strconv.ParseFloat(o.ArtPreis, 64)
		d := Dish{Date: o.Datum, Line: line, Name: name, Price: price,
			Allergens: legend(o.AllergeneFromTableAsString), Additives: legend(o.ZusatzstoffeAsString)}
		for _, s := range symbols {
			if o.Symbolik&s.bit != 0 {
				d.Tags = append(d.Tags, s.name)
			}
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// ── account ────────────────────────────────────────────────────────────────

type Booking struct {
	At     time.Time `json:"at"`
	Item   string    `json:"item"`           // "Menü 1", "Aufladung 50€", …
	Dish   string    `json:"dish,omitempty"` // what the menu was that day
	Qty    int       `json:"qty"`
	Amount float64   `json:"amount"` // negative = paid
}

type Account struct {
	Balance  float64   `json:"balance"`
	Card     string    `json:"card"`
	Bookings []Booking `json:"bookings"`
}

var (
	saldoRe = regexp.MustCompile(`saldo-value">([^<]+)<`)
	cardRe  = regexp.MustCompile(`(?s)Kartennummer:(?:&nbsp;)?</b>\s*<br>\s*([0-9]+)`)
	rowRe   = regexp.MustCompile(`(?s)<td class="datetime" title="([^"]*)">.*?<td class="description" title="([^"]*)">.*?<td class="menge right" title="([^"]*)">.*?<td class="price right" title="([^"]*)">`)
	kidRe   = regexp.MustCompile(`mdhistory\.php\?KID=(\d+)`)
)

// euro parses "-5,00 €" / "8,07 €"
func euro(s string) (float64, bool) {
	s = strings.NewReplacer("€", "", " ", "", " ", "", ".", "", ",", ".").Replace(html.UnescapeString(s))
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func ParseAccount(page string, zone *time.Location) (Account, error) {
	m := saldoRe.FindStringSubmatch(page)
	if m == nil {
		return Account{}, fmt.Errorf("Mensa: kein Saldo auf der Seite")
	}
	bal, ok := euro(m[1])
	if !ok {
		return Account{}, fmt.Errorf("Mensa: Saldo %q nicht lesbar", m[1])
	}
	acc := Account{Balance: bal, Bookings: []Booking{}}
	if c := cardRe.FindStringSubmatch(page); c != nil {
		acc.Card = c[1]
	}
	type hint struct {
		at   string
		dish string
	}
	var hints []hint
	for _, r := range rowRe.FindAllStringSubmatch(page, -1) {
		at, err := time.ParseInLocation("02.01.06 15:04", strings.TrimSpace(r[1]), zone)
		if err != nil {
			continue
		}
		item := clean(r[2])
		if d, ok := strings.CutPrefix(item, "Hinweis:"); ok {
			hints = append(hints, hint{r[1], strings.TrimSpace(d)})
			continue
		}
		qty, _ := strconv.Atoi(strings.TrimSpace(r[3]))
		amt, _ := euro(r[4])
		b := Booking{At: at, Item: item, Qty: qty, Amount: amt}
		for k, h := range hints {
			if h.at == r[1] {
				b.Dish = h.dish
				hints = append(hints[:k], hints[k+1:]...)
				break
			}
		}
		acc.Bookings = append(acc.Bookings, b)
	}
	return acc, nil
}

// ── spending ───────────────────────────────────────────────────────────────

type Month struct {
	Month string  `json:"month"` // YYYY-MM
	Food  float64 `json:"food"`
	Meals int     `json:"meals"`
}

type Spending struct {
	Food   float64   `json:"food"`  // food and drinks, without deposit
	Meals  int       `json:"meals"` // menus bought
	TopUps float64   `json:"top_ups"`
	Since  time.Time `json:"since,omitzero"`
	Months []Month   `json:"months"`
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func Spend(bs []Booking) Spending {
	s := Spending{Months: []Month{}}
	months := map[string]*Month{}
	for _, b := range bs {
		low := strings.ToLower(b.Item)
		if !b.At.IsZero() && (s.Since.IsZero() || b.At.Before(s.Since)) {
			s.Since = b.At
		}
		switch {
		case strings.Contains(low, "pfand"):
			continue
		case b.Amount > 0 && strings.HasPrefix(low, "aufladung"):
			s.TopUps += b.Amount
			continue
		case b.Amount >= 0:
			continue
		}
		key := b.At.Format("2006-01")
		m := months[key]
		if m == nil {
			m = &Month{Month: key}
			months[key] = m
		}
		s.Food -= b.Amount
		m.Food -= b.Amount
		if strings.HasPrefix(low, "menü") {
			s.Meals += b.Qty
			m.Meals += b.Qty
		}
	}
	for _, m := range months {
		m.Food = round2(m.Food)
		s.Months = append(s.Months, *m)
	}
	sort.Slice(s.Months, func(i, j int) bool { return s.Months[i].Month < s.Months[j].Month })
	s.Food, s.TopUps = round2(s.Food), round2(s.TopUps)
	return s
}

// ── client ─────────────────────────────────────────────────────────────────

type Client struct {
	Base, User, Pass string
	Zone             *time.Location
	http             *http.Client

	mu  sync.Mutex
	kid string // set while logged in
}

func New(base, user, pass string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{Base: strings.TrimRight(base, "/"), User: user, Pass: pass, Zone: time.Local,
		http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

func (c *Client) do(req *http.Request) (string, int, error) {
	req.Header.Set("User-Agent", "naknak (+https://github.com/Raindancer118/nak-api)")
	res, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	return string(b), res.StatusCode, err
}

func (c *Client) get(path string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, c.Base+path, nil)
	s, code, err := c.do(req)
	if err == nil && code != http.StatusOK {
		err = fmt.Errorf("Mensa: HTTP %d", code)
	}
	return s, err
}

func (c *Client) postForm(path string, v url.Values) (string, error) {
	req, _ := http.NewRequest(http.MethodPost, c.Base+path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s, code, err := c.do(req)
	if err == nil && code != http.StatusOK {
		err = fmt.Errorf("Mensa: HTTP %d", code)
	}
	return s, err
}

// Menu: the current week and up to two more (what OPC publishes).
func (c *Client) Menu(weeks int) ([]Dish, error) {
	weeks = max(1, min(weeks, 3))
	page, err := c.get("/menuplan.php?VMP")
	if err != nil {
		return nil, err
	}
	out := ParseMenu(page)
	hashes := map[string]string{}
	for _, m := range hashRe.FindAllStringSubmatch(page, -1) {
		hashes[m[1]] = m[2]
	}
	for n := 1; n < weeks; n++ {
		h, ok := hashes[strconv.Itoa(n)]
		if !ok {
			break
		}
		// only navigates: no order changes (angChanges empty, Save=0)
		p, err := c.postForm("/menuplan.php?VMP=", url.Values{"WID": {strconv.Itoa(n)}, "HASH": {h}, "Richtung": {"0"}, "Return": {"0"},
			"DID": {"4"}, "GTD": {"0"}, "DAYV": {"0"}, "Save": {"0"}, "anzAusBest": {"0"}})
		if err != nil {
			return out, nil // the current week is what matters
		}
		out = append(out, ParseMenu(p)...)
	}
	return out, nil
}

func (c *Client) login() error {
	if c.User == "" || c.Pass == "" {
		return ErrAuth
	}
	c.kid = ""
	if _, err := c.get("/"); err != nil {
		return err
	}
	if _, err := c.postForm("/api/", url.Values{"service": {"cookies"}, "action": {"cookie_consent"}, "value": {"1"}}); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"service": "login", "action": "ldap", "username": c.User, "password": c.Pass, "rememberMe": false})
	req, _ := http.NewRequest(http.MethodPost, c.Base+"/api/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s, code, err := c.do(req)
	if err != nil {
		return err
	}
	if code != http.StatusOK || strings.TrimSpace(s) != "true" {
		return ErrAuth
	}
	page, err := c.get("/")
	if err != nil {
		return err
	}
	m := kidRe.FindStringSubmatch(page)
	if m == nil {
		return ErrAuth
	}
	c.kid = m[1]
	return nil
}

func loggedOut(page string) bool { return strings.Contains(page, `id="f_kartennr"`) }

// Account: balance and bookings between from and to (inclusive days).
func (c *Client) Account(from, to time.Time) (Account, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for try := 0; try < 2; try++ {
		if c.kid == "" {
			if err := c.login(); err != nil {
				return Account{}, err
			}
		}
		page, err := c.postForm("/mdhistory.php?KID="+c.kid, url.Values{"action": {"BBAnzeigen"}, "exportType": {""},
			"bb_von": {from.Format("02.01.2006")}, "bb_bis": {to.Format("02.01.2006")},
			"bb_von_submit": {from.Format("02.01.2006")}, "bb_bis_submit": {to.Format("02.01.2006")}})
		if err != nil {
			return Account{}, err
		}
		if loggedOut(page) {
			c.kid = "" // session expired
			continue
		}
		return ParseAccount(page, c.Zone)
	}
	return Account{}, ErrAuth
}
