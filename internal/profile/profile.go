// Package profile reads and edits "Mein Profil" (TYPO3 extension
// tx_nastudentdata and friends): personal data, contact details, data/grade
// sharing, addresses, payment status, classmates and the copy balance.
//
// Edits are prepared from the live form and returned as a forms.Submission.
// Nothing is sent here; callers decide (after confirmation) to Send it.
package profile

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/drift"
	"github.com/Raindancer118/nak-api/internal/forms"
	"github.com/Raindancer118/nak-api/internal/htmlx"
	"golang.org/x/net/html"
)

const (
	MeineDatenPath    = "/mein-profil/meine-daten"
	NotenfreigabePath = "/mein-profil/notenfreigabe"
	DatenfreigabePath = "/mein-profil/meine-profildaten/datenfreigabe-verwalten"
	AdressenPath      = "/mein-profil/meine-profildaten/adressen-verwalten"
	EmailsPath        = "/mein-profil/meine-profildaten/e-mails-verwalten"
	TelefonPath       = "/mein-profil/meine-profildaten/telefonnummern-verwalten"
	ZahlungPath       = "/mein-profil/meine-profildaten/standard-titel"
	KommilitonenPath  = "/mein-profil/meine-profildaten/meine-kommilitonen"
	GuthabenPath      = "/mein-profil/mein-postfach/guthaben"
	VertiefungPath    = "/studium/bachelor/vertiefungsrichtungen"
)

// ── Meine Daten ─────────────────────────────────────────────────────────────

type Personal struct {
	Vorname         string            `json:"vorname"`
	Nachname        string            `json:"nachname"`
	Login           string            `json:"login"`
	Matrikelnr      string            `json:"matrikelnr"`
	Zenturie        string            `json:"zenturie"`
	Studiengang     string            `json:"studiengang"`
	Firma           string            `json:"firma"`
	Geburtsdatum    string            `json:"geburtsdatum"`
	Strasse         string            `json:"strasse"`
	Ort             string            `json:"ort"`
	TelefonFestnetz string            `json:"telefon_festnetz"`
	TelefonMobil    string            `json:"telefon_mobil"`
	EmailNAK        string            `json:"email_nak"`
	EmailPrivat     string            `json:"email_privat"`
	Infopost        string            `json:"infopost"`
	LebenslangeMail string            `json:"lebenslange_email"`
	All             map[string]string `json:"all"`
}

// Jahrgang derives "23" from a Zenturie like "I23a".
func (p *Personal) Jahrgang() string {
	if m := regexp.MustCompile(`\d{2}`).FindString(p.Zenturie); m != "" {
		return m
	}
	return ""
}

func FetchPersonal(c *client.Client) (*Personal, error) {
	pg, err := c.Page(MeineDatenPath)
	if err != nil {
		return nil, err
	}
	p := ParsePersonal(pg.Body)
	if p.Login == "" && p.Nachname == "" {
		return nil, drift.New(MeineDatenPath, "div.form-group with span label + .form-readonly value", pg.Body)
	}
	return p, nil
}

func ParsePersonal(body string) *Personal {
	doc := htmlx.Main(htmlx.MustParse(body))
	all := map[string]string{}
	for _, g := range htmlx.All(doc, htmlx.Class("form-group")) {
		lbl := htmlx.First(g, htmlx.Tag("span", "label"))
		val := htmlx.First(g, htmlx.Class("form-readonly"))
		if lbl == nil || val == nil {
			continue
		}
		k := strings.TrimSuffix(htmlx.Text(lbl), ":")
		all[k] = htmlx.Text(val)
	}
	return &Personal{
		Vorname: all["Vorname"], Nachname: all["Nachname"], Login: all["Login"], Matrikelnr: all["Matrikelnr."],
		Zenturie: all["Zenturie"], Studiengang: all["Studiengang"], Firma: all["Firma"], Geburtsdatum: all["Geburtsdatum"],
		Strasse: all["Straße"], Ort: all["Ort"], TelefonFestnetz: all["Telefon (Festnetz)"], TelefonMobil: all["Telefon (Mobil)"],
		EmailNAK: all["Email (NAK)"], EmailPrivat: all["Private E-Mail"], Infopost: all["Infopost"],
		LebenslangeMail: all["Lebenslange E-Mail"], All: all,
	}
}

// ── generic form loading ────────────────────────────────────────────────────

// LoadForm fetches path and returns the first form whose action contains
// actionContains (or the first form with a field named like fieldKey).
func LoadForm(c *client.Client, path, actionContains string) (*forms.Form, *html.Node, error) {
	pg, err := c.Page(path)
	if err != nil {
		return nil, nil, err
	}
	doc := htmlx.MustParse(pg.Body)
	f := forms.FindByAction(forms.Parse(htmlx.Main(doc), c.Base), actionContains)
	if f == nil {
		return nil, doc, drift.New(path, fmt.Sprintf("form with action containing %q", actionContains), pg.Body)
	}
	return f, doc, nil
}

func formFrom(body, base, actionContains string) *forms.Form {
	return forms.FindByAction(forms.Parse(htmlx.Main(htmlx.MustParse(body)), base), actionContains)
}

func optLabel(f *forms.Field) string {
	for _, o := range f.Options {
		if o.Value == f.Value {
			return o.Label
		}
	}
	return f.Value
}

// ── Noten- und Anmeldungsfreigabe für den Betrieb ───────────────────────────

type Freigabe struct {
	Noten       bool `json:"noten_fuer_betrieb_freigegeben"`
	Anmeldungen bool `json:"pruefungsanmeldungen_fuer_betrieb_freigegeben"`
}

const notenAction = "action]=betriebsfreigabe"

func ParseFreigabe(body, base string) (*Freigabe, error) {
	f := formFrom(body, base, notenAction)
	if f == nil {
		return nil, drift.New(NotenfreigabePath, "form action=betriebsfreigabe", body)
	}
	n, a := f.Field("notenfreigabe"), f.Field("anmeldungfreigabe")
	if n == nil || a == nil {
		return nil, drift.New(NotenfreigabePath, "selects notenfreigabe/anmeldungfreigabe", body)
	}
	return &Freigabe{Noten: n.Value == "1", Anmeldungen: a.Value == "1"}, nil
}

func FetchFreigabe(c *client.Client) (*Freigabe, error) {
	pg, err := c.Page(NotenfreigabePath)
	if err != nil {
		return nil, err
	}
	return ParseFreigabe(pg.Body, c.Base)
}

// PrepareFreigabe builds the change; nil leaves a setting as is.
func PrepareFreigabe(c *client.Client, noten, anmeldungen *bool) (*forms.Submission, error) {
	f, _, err := LoadForm(c, NotenfreigabePath, notenAction)
	if err != nil {
		return nil, err
	}
	set := map[string]string{}
	if noten != nil {
		set["notenfreigabe"] = b01(*noten)
	}
	if anmeldungen != nil {
		set["anmeldungfreigabe"] = b01(*anmeldungen)
	}
	return f.Submit("", set)
}

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ── Datenfreigabe für Kommiliton:innen ──────────────────────────────────────

// Sharing levels as used by the CIS select boxes.
var SharingLevels = map[string]string{"0": "nicht anzeigen", "1": "nur meine Zenturie", "2": "alle Zenturien"}

// DatenfreigabeFields maps the user-facing key to the form field.
var DatenfreigabeFields = []string{"Name", "Firma", "Geburtsdatum", "Adresse", "Fest", "Mobil", "Mail"}

type Datenfreigabe map[string]string // field -> level label

const datenAction = "datenfreigabe-verwalten?"

func ParseDatenfreigabe(body, base string) (Datenfreigabe, error) {
	f := formFrom(body, base, datenAction)
	if f == nil {
		return nil, drift.New(DatenfreigabePath, "Datenfreigabe settings form", body)
	}
	out := Datenfreigabe{}
	for _, k := range DatenfreigabeFields {
		if fl := f.Field(k); fl != nil {
			out[k] = fmt.Sprintf("%s (%s)", fl.Value, optLabel(fl))
		}
	}
	return out, nil
}

func FetchDatenfreigabe(c *client.Client) (Datenfreigabe, error) {
	pg, err := c.Page(DatenfreigabePath)
	if err != nil {
		return nil, err
	}
	return ParseDatenfreigabe(pg.Body, c.Base)
}

// PrepareDatenfreigabe sets sharing levels ("0", "1", "2") per field.
func PrepareDatenfreigabe(c *client.Client, levels map[string]string) (*forms.Submission, error) {
	f, _, err := LoadForm(c, DatenfreigabePath, datenAction)
	if err != nil {
		return nil, err
	}
	set := map[string]string{}
	for k, v := range levels {
		if !validDatenKey(k) {
			return nil, fmt.Errorf("unknown Datenfreigabe field %q; allowed: %s", k, strings.Join(DatenfreigabeFields, ", "))
		}
		set[k] = v
	}
	return f.Submit("", set)
}

func validDatenKey(k string) bool {
	for _, x := range DatenfreigabeFields {
		if x == k {
			return true
		}
	}
	return false
}

// ── Kontakt: E-Mail & Telefon ───────────────────────────────────────────────

type Contact struct {
	EmailPrivat     string `json:"email_privat"`
	Infopost        bool   `json:"infopost"`
	TelefonFestnetz string `json:"telefon_festnetz"`
	TelefonMobil    string `json:"telefon_mobil"`
	Fax             string `json:"fax"`
}

const (
	emailAction = "action]=email"
	phoneAction = "action]=telefon"
)

func ParseEmail(body, base string) (string, bool, error) {
	f := formFrom(body, base, emailAction)
	if f == nil {
		return "", false, drift.New(EmailsPath, "form action=email", body)
	}
	e, ip := f.Field("email.privat"), f.Field("email.infopost")
	if e == nil {
		return "", false, drift.New(EmailsPath, "field email[privat]", body)
	}
	return e.Value, ip != nil && ip.Checked, nil
}

func ParsePhone(body, base string) (fest, mobil, fax string, err error) {
	f := formFrom(body, base, phoneAction)
	if f == nil {
		return "", "", "", drift.New(TelefonPath, "form action=telefon", body)
	}
	get := func(k string) string {
		if fl := f.Field(k); fl != nil {
			return fl.Value
		}
		return ""
	}
	return get("telefon.fest"), get("telefon.mobil"), get("telefon.fax"), nil
}

func FetchContact(c *client.Client) (*Contact, error) {
	pe, err := c.Page(EmailsPath)
	if err != nil {
		return nil, err
	}
	pt, err := c.Page(TelefonPath)
	if err != nil {
		return nil, err
	}
	ct := &Contact{}
	if ct.EmailPrivat, ct.Infopost, err = ParseEmail(pe.Body, c.Base); err != nil {
		return nil, err
	}
	if ct.TelefonFestnetz, ct.TelefonMobil, ct.Fax, err = ParsePhone(pt.Body, c.Base); err != nil {
		return nil, err
	}
	return ct, nil
}

// PrepareEmail changes the private e-mail (the CIS wants lower case) and/or
// the Infopost opt-in.
func PrepareEmail(c *client.Client, email *string, infopost *bool) (*forms.Submission, error) {
	set := map[string]string{}
	if email != nil {
		e := strings.ToLower(strings.TrimSpace(*email))
		if a, err := mail.ParseAddress(e); err != nil || a.Address != e {
			return nil, fmt.Errorf("invalid e-mail address %q", *email)
		}
		set["email.privat"] = e
	}
	if infopost != nil {
		if *infopost {
			set["email.infopost"] = "1"
		} else {
			set["email.infopost"] = ""
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("nothing to change")
	}
	f, _, err := LoadForm(c, EmailsPath, emailAction)
	if err != nil {
		return nil, err
	}
	return f.Submit("", set)
}

var phoneRe = regexp.MustCompile(`^[0-9+()/\- ]{0,30}$`)

// PreparePhone changes phone numbers; nil keeps a value, "" clears it.
func PreparePhone(c *client.Client, fest, mobil, fax *string) (*forms.Submission, error) {
	set := map[string]string{}
	for k, v := range map[string]*string{"telefon.fest": fest, "telefon.mobil": mobil, "telefon.fax": fax} {
		if v == nil {
			continue
		}
		s := strings.TrimSpace(*v)
		if !phoneRe.MatchString(s) {
			return nil, fmt.Errorf("invalid phone number %q (format: vorwahl-rufnummer)", *v)
		}
		set[k] = s
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("nothing to change")
	}
	f, _, err := LoadForm(c, TelefonPath, phoneAction)
	if err != nil {
		return nil, err
	}
	return f.Submit("", set)
}

// ── Adressen ────────────────────────────────────────────────────────────────

type Address struct {
	TypeID     string            `json:"type_id"`
	TypeName   string            `json:"type_name"`
	Types      map[string]string `json:"available_types"`
	LetterForm string            `json:"letter_form"` // as printed in letters
	Zusatz     string            `json:"adresszusatz"`
	Strasse    string            `json:"strasse"`
	PLZ        string            `json:"plz"`
	Ort        string            `json:"ort"`
}

const (
	addrChangeAction = "action]=address"
)

func ParseAddress(body, base string) (*Address, error) {
	doc := htmlx.Main(htmlx.MustParse(body))
	fs := forms.Parse(doc, base)
	a := &Address{Types: map[string]string{}}
	if tf := forms.FindByField(fs, "addressType.typeId"); tf != nil {
		fl := tf.Field("addressType.typeId")
		a.TypeID, a.TypeName = fl.Value, optLabel(fl)
		for _, o := range fl.Options {
			a.Types[o.Value] = o.Label
		}
	}
	cf := forms.FindByAction(fs, addrChangeAction)
	if cf == nil {
		return nil, drift.New(AdressenPath, "form action=address", body)
	}
	get := func(k string) string {
		if fl := cf.Field(k); fl != nil {
			return fl.Value
		}
		return ""
	}
	a.Zusatz, a.Strasse, a.PLZ, a.Ort = get("address.additionalAddress"), get("address.street"), get("address.zipCode"), get("address.city")
	if h := htmlx.First(doc, func(n *html.Node) bool {
		return htmlx.Tag("h2", "h3")(n) && strings.HasPrefix(htmlx.Text(n), "Adresse wie sie")
	}); h != nil {
		// The letter form is loose text + <br> between this heading and the next.
		var sb strings.Builder
		for s := h.NextSibling; s != nil && !htmlx.Tag("h2", "h3", "form")(s); s = s.NextSibling {
			sb.WriteString(htmlx.RawText(s))
			if htmlx.Tag("br")(s) {
				sb.WriteString("\n")
			}
		}
		var lines []string
		for _, l := range strings.Split(sb.String(), "\n") {
			if l = htmlx.Collapse(l); l != "" && !strings.HasPrefix(l, "Ist keine Adresse") {
				lines = append(lines, l)
			}
		}
		a.LetterForm = strings.Join(lines, ", ")
	}
	return a, nil
}

// addressPage returns the address page for a type ("" = default/main).
func addressPage(c *client.Client, typeID string) (string, error) {
	pg, err := c.Page(AdressenPath)
	if err != nil {
		return "", err
	}
	if typeID == "" {
		return pg.Body, nil
	}
	fs := forms.Parse(htmlx.Main(htmlx.MustParse(pg.Body)), c.Base)
	tf := forms.FindByField(fs, "addressType.typeId")
	if tf == nil {
		return "", fmt.Errorf("address type selector not found")
	}
	if cur := tf.Field("addressType.typeId"); cur != nil && cur.Value == typeID {
		return pg.Body, nil
	}
	s, err := tf.Submit("", map[string]string{"addressType.typeId": typeID})
	if err != nil {
		return "", err
	}
	// Switching the displayed type only renders another address — a read.
	p2, err := c.PostRead(s.URL, s.Values())
	if err != nil {
		return "", err
	}
	return p2.Body, nil
}

func FetchAddress(c *client.Client, typeID string) (*Address, error) {
	body, err := addressPage(c, typeID)
	if err != nil {
		return nil, err
	}
	return ParseAddress(body, c.Base)
}

// PrepareAddress changes (or with remove=true deletes) the address of a type.
func PrepareAddress(c *client.Client, typeID string, zusatz, strasse, plz, ort *string, remove bool) (*forms.Submission, error) {
	body, err := addressPage(c, typeID)
	if err != nil {
		return nil, err
	}
	f := formFrom(body, c.Base, addrChangeAction)
	if f == nil {
		return nil, fmt.Errorf("address change form not found")
	}
	if remove {
		return f.Submit("address.delete", nil)
	}
	set := map[string]string{}
	for k, v := range map[string]*string{"address.additionalAddress": zusatz, "address.street": strasse, "address.zipCode": plz, "address.city": ort} {
		if v != nil {
			set[k] = strings.TrimSpace(*v)
		}
	}
	if v, ok := set["address.zipCode"]; ok && !regexp.MustCompile(`^\d{4,5}$`).MatchString(v) {
		return nil, fmt.Errorf("invalid PLZ %q", v)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("nothing to change")
	}
	return f.Submit("address.change", set)
}

// ── Zahlungsinformation (read only) ─────────────────────────────────────────

type Payment struct {
	MandateGiven bool   `json:"sepa_mandat_erteilt"`
	IBANMasked   string `json:"iban_masked,omitempty"`
	Bank         string `json:"bank,omitempty"`
	Kontoinhaber string `json:"kontoinhaber,omitempty"`
}

// ParsePayment reports the SEPA status. The IBAN is masked; this package
// deliberately offers no way to change banking data.
func ParsePayment(body, base string) (*Payment, error) {
	f := formFrom(body, base, "SepaMandat")
	if f == nil {
		f = formFrom(body, base, "sepamandat")
	}
	if f == nil {
		return nil, drift.New(ZahlungPath, "SEPA mandate form", body)
	}
	p := &Payment{}
	if fl := f.Field("iban"); fl != nil && fl.Value != "" {
		v := strings.ReplaceAll(fl.Value, " ", "")
		if len(v) > 4 {
			p.IBANMasked = strings.Repeat("•", len(v)-4) + v[len(v)-4:]
		}
	}
	if fl := f.Field("bank"); fl != nil {
		p.Bank = fl.Value
	}
	if fl := f.Field("kontoinhaber"); fl != nil {
		p.Kontoinhaber = fl.Value
	}
	if fl := f.Field("mandatErteilt"); fl != nil {
		p.MandateGiven = fl.Checked
	}
	p.MandateGiven = p.MandateGiven || p.IBANMasked != ""
	return p, nil
}

func FetchPayment(c *client.Client) (*Payment, error) {
	pg, err := c.Page(ZahlungPath)
	if err != nil {
		return nil, err
	}
	return ParsePayment(pg.Body, c.Base)
}

// ── Kommiliton:innen ────────────────────────────────────────────────────────

// Classmate deliberately carries only name and company: birthdays, phone
// numbers and addresses of other students are not exported (DSGVO).
type Classmate struct {
	Name    string `json:"name"`
	Betrieb string `json:"betrieb,omitempty"`
}

type Classmates struct {
	Zenturie  string            `json:"zenturie"`
	Zenturien map[string]string `json:"available_zenturien,omitempty"`
	People    []Classmate       `json:"people"`
}

const zenturieAction = "nameinezenturie[action]=list"

func ParseClassmates(body, base string) *Classmates {
	doc := htmlx.Main(htmlx.MustParse(body))
	cm := &Classmates{Zenturien: map[string]string{}}
	if f := formFrom(body, base, zenturieAction); f != nil {
		if fl := f.Field("zenturie"); fl != nil {
			for _, o := range fl.Options {
				cm.Zenturien[o.Label] = o.Value
				if o.Selected {
					cm.Zenturie = o.Label
				}
			}
		}
	}
	// The desktop table carries one row per person: Bild | Name | Betrieb | …
	seen := map[string]bool{}
	for _, tbl := range htmlx.All(doc, htmlx.Tag("table")) {
		for _, tr := range htmlx.Rows(tbl) {
			cells := htmlx.Cells(tr)
			if len(cells) < 3 {
				continue
			}
			name := htmlx.Text(cells[1])
			if name == "" || name == "Name" || seen[name] {
				continue
			}
			seen[name] = true
			cm.People = append(cm.People, Classmate{Name: name, Betrieb: htmlx.Text(cells[2])})
		}
	}
	return cm
}

// FetchClassmates lists a Zenturie ("" = own). Choosing a Zenturie posts the
// filter form, which only changes what is displayed.
func FetchClassmates(c *client.Client, zenturie string) (*Classmates, error) {
	pg, err := c.Page(KommilitonenPath)
	if err != nil {
		return nil, err
	}
	cm := ParseClassmates(pg.Body, c.Base)
	if zenturie == "" || strings.EqualFold(zenturie, cm.Zenturie) {
		return cm, nil
	}
	id := ""
	for label, v := range cm.Zenturien {
		if strings.EqualFold(label, zenturie) {
			id = v
		}
	}
	if id == "" {
		return nil, fmt.Errorf("unknown Zenturie %q", zenturie)
	}
	f := formFrom(pg.Body, c.Base, zenturieAction)
	s, err := f.Submit("", map[string]string{"zenturie": id})
	if err != nil {
		return nil, err
	}
	p2, err := c.PostRead(s.URL, s.Values())
	if err != nil {
		return nil, err
	}
	return ParseClassmates(p2.Body, c.Base), nil
}

// ── Guthaben ────────────────────────────────────────────────────────────────

var balanceRe = regexp.MustCompile(`([A-Za-zäöüÄÖÜ]+guthaben):\s*([-\d.,]+\s*€)`)

func ParseBalance(body string) map[string]string {
	out := map[string]string{}
	for _, m := range balanceRe.FindAllStringSubmatch(htmlx.Text(htmlx.Main(htmlx.MustParse(body))), -1) {
		out[m[1]] = m[2]
	}
	return out
}

func FetchBalance(c *client.Client) (map[string]string, error) {
	pg, err := c.Page(GuthabenPath)
	if err != nil {
		return nil, err
	}
	return ParseBalance(pg.Body), nil
}

// ── Vertiefungsrichtung (bis Jahrgang 2023) ─────────────────────────────────

type Vertiefung struct {
	Current string            `json:"current"`
	Options map[string]string `json:"options"` // value -> label
}

const vertiefungAction = "action]=handleForm"

func ParseVertiefung(body, base string) (*Vertiefung, error) {
	f := formFrom(body, base, vertiefungAction)
	if f == nil {
		return nil, drift.New(VertiefungPath, "form action=handleForm", body)
	}
	fl := f.Field("vertiefungsrichtung")
	if fl == nil {
		return nil, fmt.Errorf("Vertiefungsrichtung select not found")
	}
	v := &Vertiefung{Options: map[string]string{}}
	for _, o := range fl.Options {
		v.Options[o.Value] = o.Label
	}
	v.Current = optLabel(fl)
	return v, nil
}

func FetchVertiefung(c *client.Client) (*Vertiefung, error) {
	pg, err := c.Page(VertiefungPath)
	if err != nil {
		return nil, err
	}
	return ParseVertiefung(pg.Body, c.Base)
}

// PrepareVertiefung builds the binding "Beantragen" request.
func PrepareVertiefung(c *client.Client, value string) (*forms.Submission, error) {
	f, _, err := LoadForm(c, VertiefungPath, vertiefungAction)
	if err != nil {
		return nil, err
	}
	return f.Submit("submit", map[string]string{"vertiefungsrichtung": value})
}
