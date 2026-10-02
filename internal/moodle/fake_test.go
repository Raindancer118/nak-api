package moodle

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMoodle stands in for token.php, the REST server, pluginfile.php and upload.php.
type fakeMoodle struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	responses map[string]string
	files     map[string][]byte
	calls     []url.Values
	tokenReqs int
	valid     string
	uploads   []upload
	nextItem  int64
}

type upload struct {
	name    string
	itemid  int64
	content []byte
}

const fakeUser, fakePass = "10001", "secret"

// 2026-09-29 10:00 Europe/Berlin
const fakeNow = 1790668800

var berlin, _ = time.LoadLocation("Europe/Berlin")

func newFake(t *testing.T) *fakeMoodle {
	f := &fakeMoodle{t: t, responses: map[string]string{}, files: map[string][]byte{}, valid: "tok-1", nextItem: 555000}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/token.php", f.token)
	mux.HandleFunc("/webservice/rest/server.php", f.rest)
	mux.HandleFunc("/webservice/pluginfile.php/", f.serveFile)
	mux.HandleFunc("/webservice/upload.php", f.upload)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeMoodle) on(fn, json string) *fakeMoodle {
	f.mu.Lock()
	f.responses[fn] = json
	f.mu.Unlock()
	return f
}

func (f *fakeMoodle) file(path string, b []byte) {
	p, _ := url.PathUnescape(path)
	f.files[p] = b
}

func (f *fakeMoodle) rotateToken() {
	f.mu.Lock()
	f.valid = fmt.Sprintf("tok-%d", f.tokenReqs+1)
	f.mu.Unlock()
}

func (f *fakeMoodle) last() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func (f *fakeMoodle) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.Get("wsfunction"))
	}
	return out
}

func (f *fakeMoodle) callOf(fn string) url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.Get("wsfunction") == fn {
			return c
		}
	}
	return nil
}

func send(w http.ResponseWriter, ct, body string) {
	w.Header().Set("Content-Type", ct)
	io.WriteString(w, body)
}

func (f *fakeMoodle) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	f.mu.Lock()
	f.tokenReqs++
	n := f.tokenReqs
	f.mu.Unlock()
	if r.PostForm.Get("username") == fakeUser && r.PostForm.Get("password") == fakePass && r.PostForm.Get("service") == "moodle_mobile_app" {
		f.mu.Lock()
		f.valid = fmt.Sprintf("tok-%d", n)
		tok := f.valid
		f.mu.Unlock()
		send(w, "application/json", `{"token":"`+tok+`","privatetoken":"priv"}`)
		return
	}
	send(w, "application/json", `{"error":"Ungültige Anmeldedaten","errorcode":"invalidlogin","stacktrace":null}`)
}

func (f *fakeMoodle) rest(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	f.mu.Lock()
	f.calls = append(f.calls, r.PostForm)
	valid := f.valid
	body, ok := f.responses[r.PostForm.Get("wsfunction")]
	f.mu.Unlock()
	if r.PostForm.Get("wstoken") != valid {
		send(w, "application/json", `{"exception":"core\\exception\\moodle_exception","errorcode":"invalidtoken","message":"Ungültiges Token"}`)
		return
	}
	if !ok {
		send(w, "application/json", `{"exception":"webservice_access_exception","errorcode":"accessexception","message":"Access control exception"}`)
		return
	}
	send(w, "application/json", body)
}

func (f *fakeMoodle) serveFile(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	valid := f.valid
	f.mu.Unlock()
	if r.URL.Query().Get("token") != valid {
		send(w, "application/json; charset=utf-8", `{"error":"Ungültiges Token","errorcode":"invalidtoken"}`)
		return
	}
	b, ok := f.files[r.URL.Path]
	if !ok {
		w.WriteHeader(404)
		io.WriteString(w, "not found")
		return
	}
	ct := "application/pdf"
	switch {
	case strings.HasSuffix(r.URL.Path, ".html"):
		ct = "text/html; charset=utf-8"
	case strings.HasSuffix(r.URL.Path, ".zip"):
		ct = "application/zip"
	}
	w.Header().Set("Content-Type", ct)
	w.Write(b)
}

func (f *fakeMoodle) upload(w http.ResponseWriter, r *http.Request) {
	_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	mr := multipart.NewReader(r.Body, params["boundary"])
	fields := map[string]string{}
	var files []upload
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p)
		if p.FileName() != "" {
			files = append(files, upload{name: p.FileName(), content: b})
		} else {
			fields[p.FormName()] = string(b)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if fields["token"] != f.valid {
		send(w, "application/json", `{"error":"Ungültiges Token","errorcode":"invalidtoken"}`)
		return
	}
	f.nextItem++
	item := f.nextItem
	var parts []string
	for _, u := range files {
		u.itemid = item
		f.uploads = append(f.uploads, u)
		parts = append(parts, fmt.Sprintf(`{"filename":%q,"itemid":%d,"filearea":"draft"}`, u.name, item))
	}
	send(w, "application/json", "["+strings.Join(parts, ",")+"]")
}

func (f *fakeMoodle) client() *Client { return NewClient(f.srv.URL, fakeUser, fakePass) }

func (f *fakeMoodle) service() *Service {
	s := NewService(f.client(), berlin)
	s.Now = func() time.Time { return time.Unix(fakeNow, 0) }
	return s
}

// minimalPDF builds a valid PDF with one Helvetica text line per page.
func minimalPDF(pages ...string) []byte {
	var objs []string
	n := len(pages)
	kids := make([]string, n)
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 3+2*i)
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	fontObj := 3 + 2*n
	for i, text := range pages {
		stream := fmt.Sprintf("BT /F1 12 Tf 50 700 Td (%s) Tj ET", text)
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>", 4+2*i, fontObj))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return []byte(b.String())
}
