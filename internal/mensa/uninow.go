package mensa

// UniNow serves the menu the "NORDAKADEMIE App" (UniNow white label,
// com.uninow.ne) shows: a guest token from accounts.uninow.com, signed the way
// the app signs it, then the GraphQL query the app sends. Reverse engineered
// from app 3.141.5 (11/2026), since mensa.nordakademie.de went dark.

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// UniNowSecret is the app's embedded secret; set at build time
// (-ldflags -X …/mensa.UniNowSecret=…) or via NAK_UNINOW_SECRET, never committed.
var UniNowSecret string

const (
	uniNowPackage = "com.uninow.ne"
	uniNowUni     = "de_ne"
	uniNowQuery   = `query Cafeteria($uniID: ID!) { cafeteria(uniID: $uniID) { additions { key value } closedUntil name
 meals { date title category priceStudents priceEmployees priceGuests additions vegetarian vegan } } }`
)

type UniNow struct {
	AccountsURL, GraphQLURL string
	AppVersion              string // must match the secret's app release
	Zone                    *time.Location

	secret string
	http   *http.Client
	device string

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewUniNow returns nil without a secret: then only OPC is asked.
func NewUniNow(secret string) *UniNow {
	if secret == "" {
		return nil
	}
	id := make([]byte, 16)
	rand.Read(id)
	return &UniNow{AccountsURL: "https://accounts.uninow.com", GraphQLURL: "https://graphql.uninow.com/", AppVersion: "3.141.5",
		Zone: time.Local, secret: secret, http: &http.Client{Timeout: 30 * time.Second},
		device: fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:])}
}

// uniNowVKey mirrors com.uninow.helper.b.d(): base64(sha256(gson{packageName,versionName,secret})).
func uniNowVKey(pkg, version, secret string) string {
	b, _ := json.Marshal(struct {
		PackageName string `json:"packageName"`
		VersionName string `json:"versionName"`
		Secret      string `json:"secret"`
	}{pkg, version, secret})
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (u *UniNow) post(url string, body any, hdr map[string]string) ([]byte, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := u.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("UniNow: %w", err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("UniNow: HTTP %d", res.StatusCode)
	}
	return out, nil
}

func (u *UniNow) bearer() (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.token != "" && time.Now().Before(u.expires) {
		return u.token, nil
	}
	// key order as the app's JSON.stringify: system, version, systemVersion
	ua := []byte(`{"system":"ANDROID","version":` + strconvQuote(u.AppVersion) + `,"systemVersion":"16"}`)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	m := hmac.New(sha256.New, []byte(uniNowVKey(uniNowPackage, u.AppVersion, u.secret)))
	m.Write(append(append([]byte{}, ua...), now...))
	body := map[string]any{"userAgent": json.RawMessage(ua), "applicationId": uniNowPackage, "grantType": "hmac",
		"device": map[string]any{"deviceId": u.device, "legacyId": nil, "nativeId": strings.ReplaceAll(u.device, "-", "")[:16]}}
	out, err := u.post(strings.TrimRight(u.AccountsURL, "/")+"/api/v1/oauth/token", body, map[string]string{
		"Authorization": "HMAC-SHA256", "X-Authorization-Timestamp": now,
		"X-Authorization-Content-SHA256": hex.EncodeToString(m.Sum(nil)), "X-App-Version": u.AppVersion})
	if err != nil {
		return "", fmt.Errorf("UniNow-Gastanmeldung: %w", err)
	}
	var r struct {
		Token struct {
			Token string `json:"token"`
		} `json:"token"`
	}
	if json.Unmarshal(out, &r) != nil || r.Token.Token == "" {
		return "", fmt.Errorf("UniNow-Gastanmeldung: kein Token")
	}
	u.token, u.expires = r.Token.Token, time.Now().Add(time.Hour)
	if exp := jwtExpiry(r.Token.Token); !exp.IsZero() {
		u.expires = exp.Add(-5 * time.Minute)
	}
	return u.token, nil
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func jwtExpiry(tok string) time.Time {
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}
	}
	var c struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(b, &c) != nil || c.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}

type uniNowMeal struct {
	Date          string   `json:"date"` // 2026/11/09
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	PriceStudents *float64 `json:"priceStudents"`
	Additions     []string `json:"additions"`
	Vegetarian    bool     `json:"vegetarian"`
	Vegan         bool     `json:"vegan"`
}

// Menu: dishes from the start of the current week through `weeks` weeks.
func (u *UniNow) Menu(now time.Time, weeks int) ([]Dish, error) {
	tok, err := u.bearer()
	if err != nil {
		return nil, err
	}
	out, err := u.post(u.GraphQLURL, map[string]any{"query": uniNowQuery, "variables": map[string]string{"uniID": uniNowUni}},
		map[string]string{"Authorization": "Bearer " + tok, "Apollo-Require-Preflight": "true", "locale": "de"})
	if err != nil {
		return nil, err
	}
	var r struct {
		Data struct {
			Cafeteria []struct {
				Additions []struct{ Key, Value string } `json:"additions"`
				Meals     []uniNowMeal                  `json:"meals"`
			} `json:"cafeteria"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("UniNow: %w", err)
	}
	if len(r.Errors) > 0 && len(r.Data.Cafeteria) == 0 {
		return nil, fmt.Errorf("UniNow: %s", r.Errors[0].Message)
	}
	day := now.In(u.Zone)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, u.Zone)
	from := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
		from = from.AddDate(0, 0, 7) // weekend: the coming week counts as current
	}
	to := from.AddDate(0, 0, 7*max(1, min(weeks, 3)))
	var ds []Dish
	for _, c := range r.Data.Cafeteria {
		legendOf := map[string]string{}
		for _, a := range c.Additions {
			legendOf[a.Key] = a.Value
		}
		for _, m := range c.Meals {
			t, err := time.ParseInLocation("2006/01/02", m.Date, u.Zone)
			if err != nil || t.Before(from) || !t.Before(to) {
				continue
			}
			d := Dish{Date: t.Format("2006-01-02"), Line: m.Category, Name: strings.TrimSpace(m.Title)}
			if m.PriceStudents != nil {
				d.Price = *m.PriceStudents
			}
			if m.Vegetarian || m.Vegan {
				d.Tags = append(d.Tags, "Vegetarisch")
			}
			if m.Vegan {
				d.Tags = append(d.Tags, "Vegan")
			}
			for _, k := range m.Additions {
				v := legendOf[k]
				if v == "" {
					v = k
				}
				if isAllergenKey(k) {
					d.Allergens = append(d.Allergens, strings.TrimSuffix(v, " und daraus gewonnene Erzeugnisse"))
				} else {
					d.Additives = append(d.Additives, v)
				}
			}
			ds = append(ds, d)
		}
	}
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].Date < ds[j].Date })
	return ds, nil
}

// EU allergens are letters a–n at the NAK; additives are numbers
func isAllergenKey(k string) bool { return len(k) == 1 && k[0] >= 'a' && k[0] <= 'z' }
