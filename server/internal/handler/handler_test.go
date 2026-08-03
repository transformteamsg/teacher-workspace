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

// Register's contract is which routes it wraps, so a counting stand-in for the
// session middleware is enough: what the middleware itself does is covered by
// its own tests.
func TestHandler_Register(t *testing.T) {
	t.Run("routes requests in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("proxied:" + r.URL.Path))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		tests := []struct {
			name       string
			target     string
			wantStatus int
			wantBody   string
			wantCalls  int
		}{
			{
				name:       "static asset",
				target:     "/static/js/index.abc123.js",
				wantStatus: http.StatusOK,
				wantBody:   "proxied:/static/js/index.abc123.js",
				wantCalls:  0,
			},
			{
				name:       "application route",
				target:     "/dashboard",
				wantStatus: http.StatusOK,
				wantBody:   "proxied:/dashboard",
				wantCalls:  1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var calls int
				session := func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						next.ServeHTTP(w, r)
					})
				}

				h := New(&config.Config{
					Env:          config.EnvDevelopment,
					DevServerURL: devServerURL,
				})

				mux := http.NewServeMux()
				h.Register(mux, session)

				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				if want, got := tt.wantStatus, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := tt.wantBody, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
				if want := tt.wantCalls; want != calls {
					t.Errorf("want: %d; got: %d", want, calls)
				}
			})
		}
	})

	t.Run("routes requests in production environment", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		tests := []struct {
			name       string
			target     string
			wantStatus int
			wantBody   string
			wantCalls  int
		}{
			{
				name:       "static asset",
				target:     "/static/js/index.abc123.js",
				wantStatus: http.StatusOK,
				wantBody:   "console.log('Hello world!');",
				wantCalls:  0,
			},
			{
				name:       "application route",
				target:     "/dashboard",
				wantStatus: http.StatusOK,
				wantBody:   "<html>Hello world!</html>",
				wantCalls:  1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var calls int
				session := func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						next.ServeHTTP(w, r)
					})
				}

				h := New(&config.Config{
					Env:      config.EnvProduction,
					BuildDir: buildDir,
				})

				mux := http.NewServeMux()
				h.Register(mux, session)

				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				if want, got := tt.wantStatus, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := tt.wantBody, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
				if want := tt.wantCalls; want != calls {
					t.Errorf("want: %d; got: %d", want, calls)
				}
			})
		}
	})
}
