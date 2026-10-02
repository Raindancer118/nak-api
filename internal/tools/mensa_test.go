package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Raindancer118/nak-api/internal/mensa"
)

func fakeMensa(t *testing.T) *httptest.Server {
	file := func(f string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			b, _ := os.ReadFile("../mensa/testdata/" + f)
			w.Write(b)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("true")) })
	mux.HandleFunc("/menuplan.php", file("menuplan.html"))
	mux.HandleFunc("/", file("history.html"))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMensaTools(t *testing.T) {
	e := newEnv(t)
	e.app.UseMensa(mensa.New(fakeMensa(t).URL, "12345", "pw"))
	r := All()
	call := func(name string, args map[string]any) string {
		t.Helper()
		v, err := r.Call(e.app, name, args)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	if m := call("mensa_menu", nil); !strings.Contains(m, `"weekday":"Montag"`) || !strings.Contains(m, "Hirtenrolle") {
		t.Errorf("menu %s", m)
	}
	if a := call("mensa_account", nil); !strings.Contains(a, `"balance":8.07`) || !strings.Contains(a, "Putenschnitzel") {
		t.Errorf("account %s", a)
	}
	if s := call("mensa_spending", nil); !strings.Contains(s, `"food":`) || !strings.Contains(s, `"meals":`) {
		t.Errorf("spending %s", s)
	}
}
