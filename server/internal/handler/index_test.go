package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
	"github.com/transformteamsg/teacher-workspace/server/internal/session"
)

// failingTemplate writes part of a page and then fails, as a template does
// when it errors midway through rendering.
type failingTemplate struct{}

func (failingTemplate) Execute(_ context.Context, w io.Writer, _ any) error {
	if _, err := io.WriteString(w, "<html>"); err != nil {
		return err
	}
	return errors.New("template failed")
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

		state := decodePreloadedState(t, rec.Body.String())
		if !sess.VerifyCSRFToken(state.CSRFToken) {
			t.Errorf("want: a token the session accepts; got: %q", state.CSRFToken)
		}
	})

	t.Run("offers only the registered apps the session's school is allowed", func(t *testing.T) {
		const (
			postsRemote    = `{"name":"pg","entry":"https://posts.example.com/mf-manifest.json"}`
			groupsRemote   = `{"name":"pg","entry":"https://groups.example.com/mf-manifest.json"}`
			studentsRemote = `{"name":"student_insights","entry":"https://student-insights.example.com/mf-manifest.json"}`

			schoolCode      = "1234"
			otherSchoolCode = "0001"
		)
		allowAll := func(c *config.RemoteAppsConfig) {
			c.PostsAllowedSchoolCodes, c.GroupsAllowedSchoolCodes, c.StudentsAllowedSchoolCodes = "*", "*", "*"
		}
		tests := []struct {
			name        string
			mutate      func(*config.RemoteAppsConfig)
			wantRemotes string
			wantApps    string
		}{
			{
				name:        "when every app allows every school",
				mutate:      allowAll,
				wantRemotes: "[" + postsRemote + "," + groupsRemote + "," + studentsRemote + "]",
				wantApps:    `["posts","groups","students"]`,
			},
			{
				name:        "when only Posts allows the school",
				mutate:      func(c *config.RemoteAppsConfig) { c.PostsAllowedSchoolCodes = schoolCode },
				wantRemotes: "[" + postsRemote + "]",
				wantApps:    `["posts"]`,
			},
			{
				name:        "when only Groups allows the school",
				mutate:      func(c *config.RemoteAppsConfig) { c.GroupsAllowedSchoolCodes = schoolCode },
				wantRemotes: "[" + groupsRemote + "]",
				wantApps:    `["groups"]`,
			},
			{
				name:        "when only Student Insights allows the school",
				mutate:      func(c *config.RemoteAppsConfig) { c.StudentsAllowedSchoolCodes = schoolCode },
				wantRemotes: "[" + studentsRemote + "]",
				wantApps:    `["students"]`,
			},
			{
				name: "when no list includes the school",
				mutate: func(c *config.RemoteAppsConfig) {
					c.PostsAllowedSchoolCodes, c.GroupsAllowedSchoolCodes, c.StudentsAllowedSchoolCodes = otherSchoolCode, otherSchoolCode, otherSchoolCode
				},
				wantRemotes: `[]`,
				wantApps:    `[]`,
			},
			{
				name:        "when every list is unset",
				mutate:      func(c *config.RemoteAppsConfig) {},
				wantRemotes: `[]`,
				wantApps:    `[]`,
			},
			{
				name: "when Student Insights excludes the school",
				mutate: func(c *config.RemoteAppsConfig) {
					allowAll(c)
					c.StudentsAllowedSchoolCodes = "*,!" + schoolCode
				},
				wantRemotes: "[" + postsRemote + "," + groupsRemote + "]",
				wantApps:    `["posts","groups"]`,
			},
			{
				name:        "when only Meetings allows every school",
				mutate:      func(c *config.RemoteAppsConfig) { c.MeetingsAllowedSchoolCodes = "*" },
				wantRemotes: `[]`,
				wantApps:    `[]`,
			},
			{
				name: "when no app is registered",
				mutate: func(c *config.RemoteAppsConfig) {
					*c = config.RemoteAppsConfig{}
					allowAll(c)
				},
				wantRemotes: `[]`,
				wantApps:    `[]`,
			},
			{
				name: "when Student Insights is only partly registered",
				mutate: func(c *config.RemoteAppsConfig) {
					c.StudentInsightsBackendBaseURL = nil
					c.StudentsAllowedSchoolCodes = "*"
				},
				wantRemotes: `[]`,
				wantApps:    `[]`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				remoteApps := registeredRemoteApps()
				test.mutate(&remoteApps)
				h := &Handler{
					cfg:           &config.Config{Env: config.EnvProduction, RemoteApps: remoteApps},
					indexTemplate: newPreloadedStateTemplate(),
				}

				sess := session.New()
				sess.SetUser(session.User{ID: "staff-1", Email: "teacher@example.com", Role: "ROLE_TEACHER", SchoolCode: schoolCode})
				ctx := middleware.WithSession(t.Context(), sess)

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()

				h.index().ServeHTTP(rec, req)

				state := decodePreloadedState(t, rec.Body.String())
				if want, got := test.wantRemotes, string(state.Remotes); want != got {
					t.Errorf("want state.Remotes: %s; got: %s", want, got)
				}
				if want, got := test.wantApps, string(state.Apps); want != got {
					t.Errorf("want state.Apps: %s; got: %s", want, got)
				}
			})
		}
	})

	t.Run("offers no apps when the session has no user", func(t *testing.T) {
		remoteApps := registeredRemoteApps()
		remoteApps.PostsAllowedSchoolCodes, remoteApps.GroupsAllowedSchoolCodes, remoteApps.StudentsAllowedSchoolCodes = "*", "*", "*"
		h := &Handler{
			cfg:           &config.Config{Env: config.EnvProduction, RemoteApps: remoteApps},
			indexTemplate: newPreloadedStateTemplate(),
		}

		ctx := middleware.WithSession(t.Context(), session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index().ServeHTTP(rec, req)

		state := decodePreloadedState(t, rec.Body.String())
		if want, got := `[]`, string(state.Remotes); want != got {
			t.Errorf("want state.Remotes: %s; got: %s", want, got)
		}
		if want, got := `[]`, string(state.Apps); want != got {
			t.Errorf("want state.Apps: %s; got: %s", want, got)
		}
	})

	t.Run("embeds the user in preloaded state when the session is authenticated", func(t *testing.T) {
		tests := []struct {
			name       string
			schoolCode string
			want       string
		}{
			{
				name:       "with their school's name",
				schoolCode: "3015",
				want:       `{"name":"Tan Mei Ling","email":"tan_mei_ling@schools.gov.sg","role":"Teacher","school":"Whitley Secondary School"}`,
			},
			{
				name:       "without a school name when their school has no known name",
				schoolCode: "1001",
				want:       `{"name":"Tan Mei Ling","email":"tan_mei_ling@schools.gov.sg","role":"Teacher"}`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				h := &Handler{
					cfg:           &config.Config{Env: config.EnvProduction},
					indexTemplate: newPreloadedStateTemplate(),
					schoolNames:   map[string]string{"3015": "Whitley Secondary School"},
				}

				sess := session.New()
				sess.SetUser(session.User{
					ID:         "a1b2c3",
					Name:       "Tan Mei Ling",
					Email:      "tan_mei_ling@schools.gov.sg",
					Role:       "ROLE_TEACHER",
					SchoolCode: test.schoolCode,
					Attributes: []string{"ATTR_PG_USER"},
				})
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, sess)

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()

				h.index().ServeHTTP(rec, req)

				if want, got := http.StatusOK, rec.Code; want != got {
					t.Fatalf("want status: %d; got: %d", want, got)
				}

				payload := strings.TrimSuffix(strings.TrimPrefix(rec.Body.String(), preloadedStateOpenTag), preloadedStateCloseTag)
				var state struct {
					User json.RawMessage `json:"user"`
				}
				if err := json.Unmarshal([]byte(payload), &state); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if got := string(state.User); test.want != got {
					t.Errorf("want state.User: %s; got: %s", test.want, got)
				}
			})
		}
	})

	t.Run("omits the user from preloaded state when the session is unauthenticated", func(t *testing.T) {
		h := &Handler{
			cfg:           &config.Config{Env: config.EnvProduction},
			indexTemplate: newPreloadedStateTemplate(),
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index().ServeHTTP(rec, req)

		payload := strings.TrimSuffix(strings.TrimPrefix(rec.Body.String(), preloadedStateOpenTag), preloadedStateCloseTag)
		var state map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &state); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if got, ok := state["user"]; ok {
			t.Errorf("want state[\"user\"] ok: false; got: %s", got)
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

// registeredRemoteApps returns a remote apps config with Posts, Groups and Student Insights registered and no allowlist set.
func registeredRemoteApps() config.RemoteAppsConfig {
	return config.RemoteAppsConfig{
		PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
		PostsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.posts.example.com"},
		PostsBackendSigningKey: "a-string-secret-at-least-256-bits-long",

		GroupsManifestURL:       &url.URL{Scheme: "https", Host: "groups.example.com", Path: "/mf-manifest.json"},
		GroupsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.groups.example.com"},
		GroupsBackendSigningKey: "a-string-secret-at-least-256-bits-long",

		StudentInsightsManifestURL:       &url.URL{Scheme: "https", Host: "student-insights.example.com", Path: "/mf-manifest.json"},
		StudentInsightsBackendBaseURL:    &url.URL{Scheme: "https", Host: "api.student-insights.example.com"},
		StudentInsightsBackendSigningKey: "a-string-secret-at-least-256-bits-long",
	}
}
