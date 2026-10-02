package moodle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCallSendsTokenFunctionAndJSONFormat(t *testing.T) {
	f := newFake(t).on("core_webservice_get_site_info", `{"userid":4711,"username":"10001"}`)
	site, err := f.client().Call("core_webservice_get_site_info", nil)
	if err != nil || site.Get("userid").Int(0) != 4711 {
		t.Fatalf("%v %v", site, err)
	}
	c := f.last()
	if c.Get("wstoken") != "tok-1" || c.Get("moodlewsrestformat") != "json" || c.Get("wsfunction") != "core_webservice_get_site_info" {
		t.Errorf("call = %v", c)
	}
}

func TestTokenFetchedOnceAndRenewed(t *testing.T) {
	f := newFake(t).on("f", `{"ok":true}`)
	c := f.client()
	c.Call("f", nil)
	c.Call("f", nil)
	if f.tokenReqs != 1 {
		t.Fatalf("token requests = %d", f.tokenReqs)
	}
	f.rotateToken()
	r, err := c.Call("f", nil)
	if err != nil || !r.Get("ok").Bool(false) || f.tokenReqs != 2 {
		t.Fatalf("renew: %v %v reqs=%d", r, err, f.tokenReqs)
	}
}

func TestWrongCredentialsReadableAndNoPassword(t *testing.T) {
	f := newFake(t)
	_, err := NewClient(f.srv.URL, fakeUser, "nope").Call("f", nil)
	var me *Error
	if !errors.As(err, &me) || me.Code != "invalidlogin" || strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
}

func TestMoodleExceptionsAreMapped(t *testing.T) {
	f := newFake(t).on("core_course_get_contents", `{"exception":"dml_missing_record_exception","errorcode":"invalidrecord","message":"Can't find data record"}`)
	_, err := f.client().Call("core_course_get_contents", map[string]any{"courseid": 1})
	var me *Error
	if !errors.As(err, &me) || me.Code != "invalidrecord" || !strings.Contains(err.Error(), "Can't find data record") {
		t.Fatalf("err = %v", err)
	}
}

func TestFlatten(t *testing.T) {
	flat := Flatten(map[string]any{"courseids": []int64{1, 2}, "options": []map[string]any{{"name": "x", "value": true}}, "userid": 4711,
		"nested": map[string]any{"a": []any{"p", 2.0}}})
	want := map[string]string{"courseids[0]": "1", "courseids[1]": "2", "options[0][name]": "x", "options[0][value]": "1", "userid": "4711",
		"nested[a][0]": "p", "nested[a][1]": "2"}
	for k, v := range want {
		if flat[k] != v {
			t.Errorf("%s = %q, want %q", k, flat[k], v)
		}
	}
}

func TestDownloads(t *testing.T) {
	f := newFake(t).on("f", "[]")
	c := f.client()
	c.Call("f", nil)
	dir := t.TempDir()
	pdf := []byte("%PDF-1.7 fake")
	f.file("/webservice/pluginfile.php/1/mod_resource/content/1/Folien%20Kap%201.pdf", pdf)
	p, _, err := c.Download(f.srv.URL+"/webservice/pluginfile.php/1/mod_resource/content/1/Folien%20Kap%201.pdf?forcedownload=1", dir)
	if err != nil || filepath.Base(p) != "Folien Kap 1.pdf" {
		t.Fatalf("download: %s %v", p, err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, pdf) {
		t.Error("content differs")
	}

	// never overwrite
	f.file("/webservice/pluginfile.php/1/a.pdf", []byte{1})
	os.WriteFile(filepath.Join(dir, "a.pdf"), []byte{9}, 0o644)
	p, _, err = c.Download(f.srv.URL+"/webservice/pluginfile.php/1/a.pdf", dir)
	if err != nil || filepath.Base(p) != "a (1).pdf" {
		t.Fatalf("no-overwrite: %s %v", p, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.pdf")); b[0] != 9 {
		t.Error("existing file overwritten")
	}

	// plain pluginfile URLs are routed through the web service
	f.file("/webservice/pluginfile.php/1/b.pdf", []byte{2})
	if _, _, err := c.Download(f.srv.URL+"/pluginfile.php/1/b.pdf", dir); err != nil {
		t.Errorf("plain pluginfile: %v", err)
	}
	// foreign hosts rejected
	if _, _, err := c.Download("https://evil.example/webservice/pluginfile.php/1/a.pdf", dir); err == nil {
		t.Error("foreign host accepted")
	}
	// traversal in file name is neutralised
	f.file("/webservice/pluginfile.php/1/..%2F..%2Fx.pdf", []byte{3})
	p, _, err = c.Download(f.srv.URL+"/webservice/pluginfile.php/1/..%2F..%2Fx.pdf", dir)
	if err != nil || filepath.Dir(p) != dir {
		t.Errorf("traversal: %s %v", p, err)
	}
	// missing file is an error
	if _, _, err := c.Download(f.srv.URL+"/webservice/pluginfile.php/1/missing.pdf", dir); err == nil {
		t.Error("missing file accepted")
	}
}

func TestDownloadRenewsExpiredToken(t *testing.T) {
	f := newFake(t).on("f", "[]")
	c := f.client()
	c.Call("f", nil)
	f.rotateToken()
	f.file("/webservice/pluginfile.php/1/c.pdf", []byte{4})
	if _, _, err := c.Download(f.srv.URL+"/webservice/pluginfile.php/1/c.pdf", t.TempDir()); err != nil || f.tokenReqs != 2 {
		t.Fatalf("%v reqs=%d", err, f.tokenReqs)
	}
}

func TestReadOnlyBlocksWritesAndUploads(t *testing.T) {
	f := newFake(t).on("mod_forum_add_discussion_post", `{"postid":1}`)
	c := f.client()
	c.ReadOnly = true
	if _, err := c.CallWrite("mod_forum_add_discussion_post", map[string]any{"postid": 1}); err != ErrReadOnly {
		t.Errorf("CallWrite = %v", err)
	}
	if _, err := c.UploadDraft([]string{"x"}); err != ErrReadOnly {
		t.Errorf("UploadDraft = %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("server called: %v", f.called())
	}
}

func TestWritesAreAudited(t *testing.T) {
	f := newFake(t).on("mod_forum_add_discussion_post", `{"postid":1}`)
	c := f.client()
	c.AuditDir = t.TempDir()
	if _, err := c.CallWrite("mod_forum_add_discussion_post", map[string]any{"postid": 1, "message": "geheim"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(c.AuditDir, "writes.log"))
	if !strings.Contains(string(b), "moodle WS mod_forum_add_discussion_post fields=message,postid") || strings.Contains(string(b), "geheim") {
		t.Errorf("audit = %q", b)
	}
}
