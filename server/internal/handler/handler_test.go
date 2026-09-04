package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
)

func TestHandler_Register(t *testing.T) {
	t.Run("routes to the handler matching the request path", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		postsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("posts:" + r.URL.RequestURI()))
		}))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir
		cfg.APIProxy.PostsBaseURL = postsBackendURL
		cfg.Remotes = "pg=https://pg.test/mf-manifest.json"

		h := New(&cfg)

		mux := http.NewServeMux()
		h.Register(mux, func(next http.Handler) http.Handler { return next })

		tests := []struct {
			name     string
			target   string
			wantCode int
			wantBody string
		}{
			{name: "index", target: "/", wantCode: http.StatusOK, wantBody: "<html>Hello world!</html>"},
			{name: "static asset", target: "/static/js/index.abc123.js", wantCode: http.StatusOK, wantBody: "console.log('Hello world!');"},
			{name: "runtime config", target: "/config.json", wantCode: http.StatusOK, wantBody: "{\"remotes\":[{\"name\":\"pg\",\"entry\":\"https://pg.test/mf-manifest.json\"}]}\n"},
			{name: "API", target: "/api/posts/hello", wantCode: http.StatusOK, wantBody: "posts:/hello"},
			{name: "API path naming no app", target: "/api/", wantCode: http.StatusNotFound, wantBody: "{\"message\":\"Not Found\"}\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				if want, got := tt.wantCode, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := tt.wantBody, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("runs application routes through the session middleware", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		postsBackend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir
		cfg.APIProxy.PostsBaseURL = postsBackendURL

		h := New(&cfg)

		tests := []struct {
			name   string
			target string
		}{
			{name: "index", target: "/"},
			{name: "API", target: "/api/posts/hello"},
			{name: "API path naming no app", target: "/api/"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var calls int

				mux := http.NewServeMux()
				h.Register(mux, func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						next.ServeHTTP(w, r)
					})
				})

				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				// More than one call means the middleware was layered per route
				// rather than once around the sub-mux.
				if want := 1; want != calls {
					t.Errorf("want: %d; got: %d", want, calls)
				}
			})
		}
	})

	t.Run("serves static assets and runtime config without the session middleware", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir

		h := New(&cfg)

		tests := []struct {
			name   string
			target string
		}{
			{name: "static asset", target: "/static/js/index.abc123.js"},
			{name: "runtime config", target: "/config.json"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var calls int

				mux := http.NewServeMux()
				h.Register(mux, func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						next.ServeHTTP(w, r)
					})
				})

				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				if want, got := http.StatusOK, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want := 0; want != calls {
					t.Errorf("want: %d; got: %d", want, calls)
				}
			})
		}
	})
}
