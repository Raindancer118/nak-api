package tools

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/eduvault"
)

func fakeEduVault(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/mcp/session", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"session":"s","expires_in":7200}`)
	})
	mux.HandleFunc("GET /api/storage/search", func(w http.ResponseWriter, r *http.Request) {
		res := []map[string]any{}
		if strings.HasPrefix(r.URL.Query().Get("modul"), "Datenbanksysteme") {
			res = append(res, map[string]any{"id": "33333333-3333-3333-3333-333333333333", "modul": "Datenbanksysteme", "type": "exam", "jahr": "2023", "original_name": "db.pdf"})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": res, "total": len(res)})
	})
	mux.HandleFunc("GET /api/storage/file/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="db 2023.pdf"`)
		io.WriteString(w, "%PDF-1.7")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func withEduVault(t *testing.T, a *app.App, url string) {
	t.Helper()
	a.ConfigDir = t.TempDir()
	secret := "JBSWY3DPEHPK3PXP"
	if _, err := eduvault.TOTP(secret, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSettings(app.Settings{EduVault: app.EduVaultSettings{URL: url, Token: "evm_x_y", Secret: secret}}); err != nil {
		t.Fatal(err)
	}
}

func TestEduVaultTools(t *testing.T) {
	e := newEnv(t)
	withEduVault(t, e.app, fakeEduVault(t).URL)
	reg := All()

	res, err := reg.Call(e.app, "eduvault_module_exams", map[string]any{"title": "Datenbanksysteme"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), "33333333") {
		t.Fatalf("module exams: %s", b)
	}
	// by module number: the title comes from the CIS (fixture grades)
	if _, err := reg.Call(e.app, "eduvault_module_exams", map[string]any{"module_nr": "Z999"}); err == nil {
		t.Fatal("unknown module number accepted")
	}

	dl, err := reg.Call(e.app, "eduvault_download", map[string]any{"exam_id": "33333333-3333-3333-3333-333333333333"})
	if err != nil {
		t.Fatal(err)
	}
	p := dl.(map[string]any)["path"].(string)
	if data, _ := os.ReadFile(p); string(data) != "%PDF-1.7" || !strings.HasSuffix(p, "db 2023.pdf") {
		t.Fatalf("download %s %q", p, data)
	}
}

func TestEduVaultNotConfigured(t *testing.T) {
	e := newEnv(t)
	e.app.ConfigDir = t.TempDir()
	t.Setenv("EDUVAULT_TOKEN", "")
	t.Setenv("EDUVAULT_MCP_SECRET", "")
	_, err := All().Call(e.app, "eduvault_search", map[string]any{"q": "x"})
	if err == nil || !strings.Contains(err.Error(), "Einstellungen") {
		t.Fatalf("err = %v", err)
	}
}
