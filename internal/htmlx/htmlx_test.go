package htmlx

import "testing"

const doc = `<html><body><main>
<div class="a b"><h2> Titel  <b>fett</b></h2>
<a href="/x?tx_ext_plugin%5Baction%5D=show&amp;tx_ext_plugin%5Bid%5D=42&amp;cHash=abc">Link</a>
<a href="https://cis.nordakademie.de/abs">Abs</a>
<table><tr><td> 1 </td><td>zwei<br/>Zeilen</td></tr></table>
</div></main></body></html>`

func TestTextCollapsesWhitespace(t *testing.T) {
	n := MustParse(doc)
	h := First(n, Tag("h2"))
	if got := Text(h); got != "Titel fett" {
		t.Fatalf("Text = %q", got)
	}
}

func TestHasClass(t *testing.T) {
	n := MustParse(doc)
	if First(n, Class("b")) == nil || First(n, Class("ab")) != nil {
		t.Fatal("class matching wrong")
	}
}

func TestQueryParamBracketNotation(t *testing.T) {
	href := Attr(First(MustParse(doc), Tag("a")), "href")
	if QueryParam(href, "action") != "show" || QueryParam(href, "id") != "42" || QueryParam(href, "cHash") != "abc" {
		t.Fatalf("params wrong for %s", href)
	}
}

func TestAbsURL(t *testing.T) {
	base := "https://cis.example"
	cases := map[string]string{
		"/x?a=1&amp;b=2":                "https://cis.example/x?a=1&b=2",
		"rel/path":                      "https://cis.example/rel/path",
		"https://cis.nordakademie.de/a": "https://cis.nordakademie.de/a",
		"":                              "",
	}
	for in, want := range cases {
		if got := AbsURL(base, in); got != want {
			t.Errorf("AbsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCellsAndLinesText(t *testing.T) {
	n := MustParse(doc)
	cells := Cells(First(n, Tag("tr")))
	if len(cells) != 2 || Text(cells[0]) != "1" {
		t.Fatalf("cells = %d", len(cells))
	}
	if got := LinesText(cells[1]); got != "zwei\nZeilen" {
		t.Fatalf("LinesText = %q", got)
	}
}

func TestAllAndDirectChildren(t *testing.T) {
	n := MustParse(doc)
	if got := len(All(n, Tag("a"))); got != 2 {
		t.Fatalf("All(a) = %d", got)
	}
	if got := len(Children(First(n, Tag("tr")), Tag("td"))); got != 2 {
		t.Fatalf("Children(td) = %d", got)
	}
}
