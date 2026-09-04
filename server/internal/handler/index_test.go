package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

func TestHandler_index(t *testing.T) {
	t.Run("proxy to the dev server in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(httputil.HeaderContentType, httputil.MIMETextHTMLCharsetUTF8)
			_, _ = w.Write([]byte("proxied:" + r.URL.Path))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
		}, nil)

		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("want status code: %d, got: %d", http.StatusOK, rec.Code)
		}
		if want, got := "proxied:/dashboard", rec.Body.String(); want != got {
			t.Errorf("want body: %s, got: %s", want, got)
		}
	})

	t.Run("serve index.html for all routes in production environment", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		h := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
		}, nil)

		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("want status code: %d, got: %d", http.StatusOK, rec.Code)
		}
		if want, got := "<html>Hello world!</html>", rec.Body.String(); want != got {
			t.Errorf("want body: %s, got: %s", want, got)
		}
	})

	t.Run("return 404 for an unknown environment", func(t *testing.T) {
		h := New(&config.Config{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("want status code: %d, got: %d", http.StatusNotFound, rec.Code)
		}
	})
}

func TestHandler_static(t *testing.T) {
	t.Run("proxy to the dev server in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(httputil.HeaderContentType, httputil.MIMETextHTMLCharsetUTF8)
			_, _ = w.Write([]byte("proxied:" + r.URL.Path))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
		}, nil)

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("want status code: %d, got: %d", http.StatusOK, rec.Code)
		}
		if want, got := "proxied:/static/js/index.js", rec.Body.String(); want != got {
			t.Errorf("want body: %s, got: %s", want, got)
		}
	})

	t.Run("serve hashed asset in production environment", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		h := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
		}, nil)

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.abc123.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("want status code: %d, got: %d", http.StatusOK, rec.Code)
		}
		if want, got := "console.log('Hello world!');", rec.Body.String(); want != got {
			t.Errorf("want body: %s, got: %s", want, got)
		}
	})

	t.Run("return 404 for an unknown environment", func(t *testing.T) {
		h := New(&config.Config{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/static/js/missing.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("want status code: %d, got: %d", http.StatusNotFound, rec.Code)
		}
	})
}
