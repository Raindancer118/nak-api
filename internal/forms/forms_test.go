package forms

import (
	"strings"
	"testing"

	"github.com/Raindancer118/nak-api/internal/client"
	"github.com/Raindancer118/nak-api/internal/htmlx"
)

// Shape copied from the real CIS e-mail form (tx_nastudentdata_nastudentdataemail).
const emailForm = `<main>
<form method="post" action="/mein-profil/meine-profildaten/e-mails-verwalten?tx_x%5Baction%5D=email&amp;cHash=abc">
<input type="hidden" name="tx_x[__referrer][@action]" value="email" />
<input type="hidden" name="tx_x[__trustedProperties]" value="{&quot;email&quot;:1}hmac" />
<input type="hidden" name="tx_x[email][typ]" value="privat" />
<input type="email" name="tx_x[email][privat]" value="old@example.org" />
<input type="hidden" name="tx_x[email][infopost]" value="" /><input type="checkbox" name="tx_x[email][infopost]" value="1" checked="checked" />
<select name="tx_x[sel]"><option value="0">nein</option><option value="1" selected="selected">ja</option></select>
<input type="text" name="tx_x[dis]" value="x" disabled="disabled" />
<input type="submit" name="tx_x[submit]" value="Angaben ändern" />
<input type="submit" name="tx_x[delete]" value="Löschen" />
</form>
<form method="post" enctype="multipart/form-data" action="/up?tx_y%5Baction%5D=confirmNew">
<input type="text" name="tx_y[topic]" value="" />
<input type="file" name="tx_y[file][]" />
<input type="submit" value="jetzt beantragen" />
</form></main>`

func parse(t *testing.T) []*Form {
	t.Helper()
	fs := Parse(htmlx.MustParse(emailForm), "https://cis.example")
	if len(fs) != 2 {
		t.Fatalf("forms = %d", len(fs))
	}
	return fs
}

func kv(s *Submission) string {
	var b strings.Builder
	for _, f := range s.Fields {
		b.WriteString(f.Name + "=" + f.Value + "\n")
	}
	return b.String()
}

func TestUntouchedFormReproducesBrowserPayload(t *testing.T) {
	f := parse(t)[0]
	s, err := f.Submit("", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "tx_x[__referrer][@action]=email\n" +
		"tx_x[__trustedProperties]={\"email\":1}hmac\n" +
		"tx_x[email][typ]=privat\n" +
		"tx_x[email][privat]=old@example.org\n" +
		"tx_x[email][infopost]=\n" +
		"tx_x[email][infopost]=1\n" +
		"tx_x[sel]=1\n" +
		"tx_x[submit]=Angaben ändern\n"
	if got := kv(s); got != want {
		t.Fatalf("payload:\n%s\nwant:\n%s", got, want)
	}
	if s.URL != "https://cis.example/mein-profil/meine-profildaten/e-mails-verwalten?tx_x%5Baction%5D=email&cHash=abc" {
		t.Errorf("url = %s", s.URL)
	}
	if s.Multipart {
		t.Error("urlencoded form flagged multipart")
	}
}

func TestOverridesWithShortKeysAndCheckboxUncheck(t *testing.T) {
	f := parse(t)[0]
	s, err := f.Submit("", map[string]string{"email.privat": "new@example.org", "email.infopost": "", "sel": "0"})
	if err != nil {
		t.Fatal(err)
	}
	got := kv(s)
	for _, w := range []string{"tx_x[email][privat]=new@example.org\n", "tx_x[sel]=0\n"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	if strings.Contains(got, "tx_x[email][infopost]=1") {
		t.Errorf("checkbox still checked:\n%s", got)
	}
	if len(s.Changes) != 3 {
		t.Errorf("changes = %v", s.Changes)
	}
}

func TestValidation(t *testing.T) {
	f := parse(t)[0]
	if _, err := f.Submit("", map[string]string{"sel": "7"}); err == nil {
		t.Error("invalid select option accepted")
	}
	if _, err := f.Submit("", map[string]string{"nope": "1"}); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := f.Submit("", map[string]string{"dis": "y"}); err == nil {
		t.Error("disabled field accepted")
	}
	if _, err := f.Submit("tx_x[missing]", nil); err == nil {
		t.Error("unknown submit button accepted")
	}
}

func TestSecondSubmitButton(t *testing.T) {
	s, err := parse(t)[0].Submit("delete", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := kv(s)
	if !strings.Contains(got, "tx_x[delete]=Löschen") || strings.Contains(got, "tx_x[submit]") {
		t.Fatalf("wrong button:\n%s", got)
	}
}

func TestMultipartWithFile(t *testing.T) {
	f := parse(t)[1]
	s, err := f.Submit("", map[string]string{"topic": "Thema"}, client.FilePart{Field: "file", Filename: "a.pdf", Data: []byte("%PDF")})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Multipart || len(s.Files) != 1 || s.Files[0].Field != "tx_y[file][]" {
		t.Fatalf("multipart = %v files = %+v", s.Multipart, s.Files)
	}
	if _, err := f.Submit("", nil, client.FilePart{Field: "nofile", Filename: "x"}); err == nil {
		t.Error("unknown file field accepted")
	}
}

func TestPreviewHidesTokens(t *testing.T) {
	s, _ := parse(t)[0].Submit("", map[string]string{"email.privat": "new@example.org"})
	p := s.Preview()
	if strings.Contains(p, "hmac") || !strings.Contains(p, "new@example.org") || !strings.Contains(p, "old@example.org") {
		t.Fatalf("preview:\n%s", p)
	}
}

func TestFind(t *testing.T) {
	fs := parse(t)
	if FindByAction(fs, "confirmNew") != fs[1] || FindByAction(fs, "nothing") != nil {
		t.Fatal("FindByAction")
	}
}

// HTML spec: a GET form replaces the action's query string with its fields.
func TestGetFormReplacesActionQuery(t *testing.T) {
	doc := htmlx.MustParse(`<form method="get" action="/s?tx_a%5Baction%5D=list&amp;cHash=1"><input type="hidden" name="tx_a[x]" value="y"><select name="tx_a[q]"><option value="1">a</option><option value="2">b</option></select></form>`)
	s, err := Parse(doc, "https://cis.example")[0].Submit("", map[string]string{"q": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.QueryURL(); got != "https://cis.example/s?tx_a%5Bq%5D=2&tx_a%5Bx%5D=y" {
		t.Errorf("QueryURL = %s", got)
	}
}
