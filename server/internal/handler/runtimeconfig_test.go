package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

func TestHandler_runtimeConfig(t *testing.T) {
	t.Run("serves the configured remotes in order", func(t *testing.T) {
		cfg := config.Default()
		cfg.Host.Remotes = "pg=https://pg.test/mf-manifest.json,si=https://si.test/mf-manifest.json"
		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
		rec := httptest.NewRecorder()

		h.runtimeConfig(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if want, got := httputil.MIMEApplicationJSONCharsetUTF8, rec.Header().Get(httputil.HeaderContentType); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}

		var body runtimeConfigResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		want := []config.Remote{
			{Name: "pg", Entry: "https://pg.test/mf-manifest.json"},
			{Name: "si", Entry: "https://si.test/mf-manifest.json"},
		}
		if got := body.Remotes; !slices.Equal(want, got) {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})

	t.Run("serves an empty array rather than null when nothing is configured", func(t *testing.T) {
		cfg := config.Default()
		cfg.Host.Remotes = ""
		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
		rec := httptest.NewRecorder()

		h.runtimeConfig(rec, req)

		if want, got := "{\"remotes\":[]}\n", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("forbids caching so a restart is picked up", func(t *testing.T) {
		cfg := config.Default()
		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
		rec := httptest.NewRecorder()

		h.runtimeConfig(rec, req)

		if want, got := "no-store", rec.Header().Get("Cache-Control"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}
