package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/handler"
	"github.com/String-sg/teacher-workspace/server/pkg/require"
)

func TestNewMux_Development(t *testing.T) {
	devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "dev-server:"+r.URL.Path)
	}))
	t.Cleanup(devServer.Close)

	devServerURL, err := url.Parse(devServer.URL)
	require.NoError(t, err)

	cfg := config.Default()
	cfg.Host.DevServerURL = devServerURL
	mux := handler.NewMux(&cfg)

	t.Run("GET / is proxied to the dev server", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "dev-server:/", w.Body.String())
	})

	t.Run("GET static asset is proxied to the dev server", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/js/index.js", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "dev-server:/static/js/index.js", w.Body.String())
	})

	t.Run("POST / returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestNewMux_Production(t *testing.T) {
	buildDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>index</html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
		t.Fatalf("mkdir static/js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('hi')"), 0o644); err != nil {
		t.Fatalf("write static asset: %v", err)
	}

	cfg := config.Default()
	cfg.Env = config.EnvProduction
	cfg.Host.BuildDir = buildDir
	mux := handler.NewMux(&cfg)

	t.Run("GET / serves index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "<html>index</html>", w.Body.String())
	})

	t.Run("GET known static asset serves the file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/js/index.abc123.js", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "console.log('hi')", w.Body.String())
	})

	t.Run("GET missing static asset returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/js/missing.js", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET unknown non-asset route falls back to index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "<html>index</html>", w.Body.String())
	})

	t.Run("POST / returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
