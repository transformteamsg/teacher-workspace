package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// stubTemplate renders a fixed page, for tests that only need to know whether
// index rendered the page rather than what it rendered.
type stubTemplate struct{}

func (stubTemplate) Execute(_ context.Context, w io.Writer, _ any) error {
	_, err := io.WriteString(w, "<html>Hello world!</html>")
	return err
}

// failingTemplate writes part of a page and then fails, as a template does
// when it errors midway through rendering.
type failingTemplate struct{}

func (failingTemplate) Execute(_ context.Context, w io.Writer, _ any) error {
	if _, err := io.WriteString(w, "<html>"); err != nil {
		return err
	}
	return errors.New("template failed")
}

const (
	preloadedStateOpenTag  = `<script type="application/json" id="preloaded-state">`
	preloadedStateCloseTag = `</script>`
)

// preloadedStateTemplate renders the preloaded state inside a JSON script tag,
// matching where the frontend's page embeds it.
type preloadedStateTemplate struct {
	tmpl *template.Template
}

func newPreloadedStateTemplate() preloadedStateTemplate {
	return preloadedStateTemplate{
		tmpl: template.Must(template.New("preloaded-state").Parse(preloadedStateOpenTag + "{{.}}" + preloadedStateCloseTag)),
	}
}

func (p preloadedStateTemplate) Execute(_ context.Context, w io.Writer, data any) error {
	return p.tmpl.Execute(w, data)
}

// newBuildDirFS creates a build directory holding one asset and a symlink to a
// file outside it, and returns it opened with os.OpenRoot.
func newBuildDirFS(t *testing.T) fs.FS {
	t.Helper()

	buildDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(buildDir, "static", "secret.txt")); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	root, err := os.OpenRoot(buildDir)
	if err != nil {
		t.Fatalf("os.OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	return root.FS()
}

func TestHandler_index(t *testing.T) {
	t.Run("embeds session CSRF token in preloaded state", func(t *testing.T) {
		h := &Handler{
			cfg:           &config.Config{Env: config.EnvProduction},
			indexTemplate: newPreloadedStateTemplate(),
		}

		sess := session.New()
		ctx := middleware.WithSession(t.Context(), sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index().ServeHTTP(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}

		payload := strings.TrimSuffix(strings.TrimPrefix(rec.Body.String(), preloadedStateOpenTag), preloadedStateCloseTag)
		var state struct {
			CSRFToken string `json:"csrfToken"`
		}
		if err := json.Unmarshal([]byte(payload), &state); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if !sess.VerifyCSRFToken(state.CSRFToken) {
			t.Errorf("want: a token the session accepts; got: %q", state.CSRFToken)
		}
	})

	t.Run("embeds configured remotes in preloaded state", func(t *testing.T) {
		tests := []struct {
			name       string
			remoteApps config.RemoteAppsConfig
			want       string
		}{
			{
				name: "all remotes configured",
				remoteApps: config.RemoteAppsConfig{
					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.posts.example.com"},
					PostsBackendSigningKey: "a-string-secret-at-least-256-bits-long",

					StudentInsightsManifestURL:       &url.URL{Scheme: "https", Host: "student-insights.example.com", Path: "/mf-manifest.json"},
					StudentInsightsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.student-insights.example.com"},
					StudentInsightsBackendSigningKey: "a-string-secret-at-least-256-bits-long",
				},
				want: `[{"name":"pg","entry":"https://posts.example.com/mf-manifest.json"},{"name":"si","entry":"https://student-insights.example.com/mf-manifest.json"}]`,
			},
			{
				name: "posts remote only",
				remoteApps: config.RemoteAppsConfig{
					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.posts.example.com"},
					PostsBackendSigningKey: "a-string-secret-at-least-256-bits-long",
				},
				want: `[{"name":"pg","entry":"https://posts.example.com/mf-manifest.json"}]`,
			},
			{
				name: "student insights remote only",
				remoteApps: config.RemoteAppsConfig{
					StudentInsightsManifestURL:       &url.URL{Scheme: "https", Host: "student-insights.example.com", Path: "/mf-manifest.json"},
					StudentInsightsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.student-insights.example.com"},
					StudentInsightsBackendSigningKey: "a-string-secret-at-least-256-bits-long",
				},
				want: `[{"name":"si","entry":"https://student-insights.example.com/mf-manifest.json"}]`,
			},
			{
				name:       "no remotes configured",
				remoteApps: config.RemoteAppsConfig{},
				want:       `[]`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				h := &Handler{
					cfg:           &config.Config{Env: config.EnvProduction, RemoteApps: test.remoteApps},
					indexTemplate: newPreloadedStateTemplate(),
				}

				ctx := middleware.WithSession(t.Context(), session.New())

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()

				h.index().ServeHTTP(rec, req)

				payload := strings.TrimSuffix(strings.TrimPrefix(rec.Body.String(), preloadedStateOpenTag), preloadedStateCloseTag)
				var state struct {
					Remotes json.RawMessage `json:"remotes"`
				}
				if err := json.Unmarshal([]byte(payload), &state); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if want, got := test.want, string(state.Remotes); want != got {
					t.Errorf("want: %s; got: %s", want, got)
				}
			})
		}
	})

	t.Run("responds with 500 and logs error when session is missing", func(t *testing.T) {
		h := &Handler{
			cfg:           &config.Config{Env: config.EnvProduction},
			indexTemplate: stubTemplate{},
		}

		var buf bytes.Buffer
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index().ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}

		var record struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := "ERROR", record.Level; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "no session found in context", record.Msg; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("responds with 500 and logs error when template fails", func(t *testing.T) {
		h := &Handler{
			cfg:           &config.Config{Env: config.EnvProduction},
			indexTemplate: failingTemplate{},
		}

		var buf bytes.Buffer
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index().ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if got := rec.Body.String(); strings.Contains(got, "<html>") {
			t.Errorf("want: not containing %q; got: %q", "<html>", got)
		}

		var record struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
			Err   string `json:"err"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := "ERROR", record.Level; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "failed to execute index template", record.Msg; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if record.Err == "" {
			t.Error("want err: non-empty; got: empty")
		}
	})
}

func TestHandler_static(t *testing.T) {
	t.Run("proxies to dev server in development", func(t *testing.T) {
		var proxiedPath string
		h := &Handler{
			cfg: &config.Config{Env: config.EnvDevelopment},
			devServerProxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				proxiedPath = r.URL.Path
			}),
		}

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if want := "/static/js/index.js"; want != proxiedPath {
			t.Errorf("want: %q; got: %q", want, proxiedPath)
		}
	})

	t.Run("serves file from build directory in production", func(t *testing.T) {
		h := &Handler{
			cfg:           &config.Config{Env: config.EnvProduction},
			prdFileSystem: newBuildDirFS(t),
		}

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.abc123.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "console.log('Hello world!');", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("responds with 404 in production", func(t *testing.T) {
		tests := []struct {
			name   string
			target string
		}{
			{
				name:   "static root",
				target: "/static/",
			},
			{
				name:   "subdirectory",
				target: "/static/js/",
			},
			{
				name:   "subdirectory without trailing slash",
				target: "/static/js",
			},
			{
				name:   "missing file",
				target: "/static/js/missing.js",
			},
			{
				name:   "symlink outside build directory",
				target: "/static/secret.txt",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				h := &Handler{
					cfg:           &config.Config{Env: config.EnvProduction},
					prdFileSystem: newBuildDirFS(t),
				}

				req := httptest.NewRequest(http.MethodGet, test.target, nil)
				rec := httptest.NewRecorder()

				h.static(rec, req)

				if want, got := http.StatusNotFound, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
			})
		}
	})

	t.Run("responds with 404 in unrecognised environment", func(t *testing.T) {
		var proxied bool
		h := &Handler{
			cfg: &config.Config{Env: "staging"},
			devServerProxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				proxied = true
			}),
		}

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if proxied {
			t.Error("want: not proxied; got: proxied")
		}
		if want, got := http.StatusNotFound, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})
}
