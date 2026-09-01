package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

func TestHandler_proxy(t *testing.T) {
	t.Run("forwards to the app's backend without the /api/<app> prefix", func(t *testing.T) {
		postsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("posts:" + r.URL.RequestURI()))
		}))
		t.Cleanup(postsBackend.Close)

		studentInsightsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("student-insights:" + r.URL.RequestURI()))
		}))
		t.Cleanup(studentInsightsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		studentInsightsBackendURL, err := url.Parse(studentInsightsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsBackendURL
		cfg.APIProxy.StudentInsightsBaseURL = studentInsightsBackendURL

		h := New(&cfg)

		tests := []struct {
			name string
			app  string
			rest string
			want string
		}{
			{name: "posts", app: "posts", rest: "/hello", want: "posts:/hello"},
			{name: "student insights", app: "student-insights", rest: "/hello", want: "student-insights:/hello"},
			{name: "nested path", app: "posts", rest: "/2026/08/hello", want: "posts:/2026/08/hello"},
			{name: "app root", app: "posts", rest: "/", want: "posts:/"},
			{name: "query string", app: "posts", rest: "/search?q=hello&page=2", want: "posts:/search?q=hello&page=2"},
			{name: "escaped path segment", app: "posts", rest: "/a%2Fb", want: "posts:/a%2Fb"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/"+tt.app+tt.rest, nil)
				req.SetPathValue("app", tt.app)
				rec := httptest.NewRecorder()

				h.proxy(rec, req)

				if want, got := http.StatusOK, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := tt.want, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			})
		}
	})

	// PG's version prefix lives on the base URL, so the rewrite must keep it.
	t.Run("keeps the base URL's path prefix", func(t *testing.T) {
		var gotPath string
		postsBackend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.RequestURI()
		}))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL + "/api/tw/1/staff")
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsBackendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/announcements?page=2", nil)
		req.SetPathValue("app", "posts")
		rec := httptest.NewRecorder()

		h.proxy(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if want, got := "/api/tw/1/staff/announcements?page=2", gotPath; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("answers 404 for an unknown app", func(t *testing.T) {
		var calls int
		count := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ })

		postsBackend := httptest.NewServer(count)
		t.Cleanup(postsBackend.Close)

		studentInsightsBackend := httptest.NewServer(count)
		t.Cleanup(studentInsightsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		studentInsightsBackendURL, err := url.Parse(studentInsightsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsBackendURL
		cfg.APIProxy.StudentInsightsBaseURL = studentInsightsBackendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/unknown-app/hello", nil)
		req.SetPathValue("app", "unknown-app")
		rec := httptest.NewRecorder()

		h.proxy(rec, req)

		if want, got := http.StatusNotFound, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want := 0; want != calls {
			t.Errorf("want: %d; got: %d", want, calls)
		}
	})

	t.Run("answers 502 when the backend is unreachable", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		// Close the backend up front so the proxy's dial is refused.
		backend.Close()

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		req.SetPathValue("app", "posts")
		rec := httptest.NewRecorder()

		h.proxy(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("strips the session cookie", func(t *testing.T) {
		var forwarded http.Header
		record := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			forwarded = r.Header.Clone()
		})

		postsBackend := httptest.NewServer(record)
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsBackendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		req.SetPathValue("app", "posts")
		req.AddCookie(&http.Cookie{Name: "session-name", Value: "session-value"})
		rec := httptest.NewRecorder()

		h.proxy(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if got := forwarded.Get("Cookie"); got != "" {
			t.Errorf("want: empty; got: %q", got)
		}
	})

	t.Run("attaches a signed JWT to outbound request", func(t *testing.T) {
		var receivedReqHeaders http.Header
		studentInsightsBackend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			receivedReqHeaders = r.Header
		}))
		t.Cleanup(studentInsightsBackend.Close)
		postsBackend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			receivedReqHeaders = r.Header
		}))
		t.Cleanup(postsBackend.Close)
		studentInsightsBackendURL, err := url.Parse(studentInsightsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.StudentInsightsBaseURL = studentInsightsBackendURL
		cfg.APIProxy.PostsBaseURL = postsBackendURL
		cfg.APIProxy.StudentInsightsSigningKey = "student-insights-string-secret-at-least-256-bits-long"
		cfg.APIProxy.PostsSigningKey = "posts-string-secret-at-least-256-bits-long"

		ttl := 2 * time.Minute
		cfg.APIProxy.TokenTTL = ttl

		// TEMP-PG-LOCAL: claims come from the env stub; seed the session instead
		// once proxy_pg_identity_stub.go goes.
		t.Setenv(envPGStubSubject, "EP000001")
		t.Setenv(envPGStubSchoolCode, "1001")

		h := New(&cfg)

		currentTime := time.Now()
		tests := []struct {
			app        string
			signingKey string
			// jwt.Claims so student-insights keeps registered claims while posts
			// carries PG's.
			into       func() jwt.Claims
			wantClaims jwt.Claims
		}{
			{
				app:        "student-insights",
				signingKey: "student-insights-string-secret-at-least-256-bits-long",
				into:       func() jwt.Claims { return &jwt.RegisteredClaims{} },
				wantClaims: &jwt.RegisteredClaims{
					Issuer:    "TW",
					Audience:  jwt.ClaimStrings{"si"},
					IssuedAt:  jwt.NewNumericDate(currentTime),
					ExpiresAt: jwt.NewNumericDate(currentTime.Add(ttl)),
				},
			},
			{
				app:        "posts",
				signingKey: "posts-string-secret-at-least-256-bits-long",
				into:       func() jwt.Claims { return &pgClaims{} },
				wantClaims: &pgClaims{
					Roles:         []string{"TW_TEACHER"},
					EffectiveRole: "TW_TEACHER",
					Attributes:    []string{"ATTR_PG_USER"},
					SchoolCode:    "1001",
					RegisteredClaims: jwt.RegisteredClaims{
						Issuer:    "TW",
						Subject:   "EP000001",
						Audience:  jwt.ClaimStrings{"pg"},
						IssuedAt:  jwt.NewNumericDate(currentTime),
						ExpiresAt: jwt.NewNumericDate(currentTime.Add(ttl)),
					},
				},
			},
		}

		for _, tt := range tests {
			req := httptest.NewRequest(http.MethodGet, "/api/"+tt.app+"/hello", nil)
			req.SetPathValue("app", tt.app)
			rec := httptest.NewRecorder()

			h.proxy(rec, req)

			if want, got := http.StatusOK, rec.Code; want != got {
				t.Errorf("want: %d; got: %d", want, got)
			}
			token, ok := strings.CutPrefix(receivedReqHeaders.Get("Authorization"), "Bearer ")
			if !ok {
				t.Fatalf("want: not empty; got: %q", receivedReqHeaders.Get("Authorization"))
			}

			claims := tt.into()
			if _, err := jwt.ParseWithClaims(token, claims,
				func(*jwt.Token) (any, error) {
					return []byte(tt.signingKey), nil
				},
				jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			); err != nil {
				t.Fatalf("want err: nil; got: %v", err)
			}

			if want, got := tt.wantClaims, claims; !reflect.DeepEqual(want, got) {
				t.Errorf("want: %+v; got: %+v", want, got)
			}
		}
	})

	// TEMP-PG-LOCAL: delete alongside proxy_pg_identity_stub.go.
	t.Run("refuses to sign a PG token in production", func(t *testing.T) {
		var calls int
		postsBackend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		// A production deployment that still had the stub env vars set would
		// otherwise hand every teacher the same identity, and the same posts.
		t.Setenv(envPGStubSubject, "EP000001")
		t.Setenv(envPGStubSchoolCode, "1001")

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.APIProxy.PostsBaseURL = postsBackendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/announcements", nil)
		req.SetPathValue("app", "posts")
		rec := httptest.NewRecorder()

		h.proxy(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want := 0; want != calls {
			t.Errorf("backend calls: want %d; got %d", want, calls)
		}
	})

	t.Run("student-insights is unaffected in production", func(t *testing.T) {
		var calls int
		siBackend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
		t.Cleanup(siBackend.Close)

		siBackendURL, err := url.Parse(siBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.APIProxy.StudentInsightsBaseURL = siBackendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/student-insights/hello", nil)
		req.SetPathValue("app", "student-insights")
		rec := httptest.NewRecorder()

		h.proxy(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want := 1; want != calls {
			t.Errorf("backend calls: want %d; got %d", want, calls)
		}
	})

	t.Run("signs PG tokens with the stubbed staff identity", func(t *testing.T) {
		var receivedReqHeaders http.Header
		postsBackend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			receivedReqHeaders = r.Header
		}))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		const signingKey = "posts-string-secret-at-least-256-bits-long"

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsBackendURL
		cfg.APIProxy.PostsSigningKey = signingKey

		h := New(&cfg)

		tests := []struct {
			name           string
			subject        string
			schoolCode     string
			roles          string
			wantRoles      []string
			wantSubject    string
			wantSchoolCode string
		}{
			{
				name:           "defaults roles when unset",
				subject:        "EP000001",
				schoolCode:     "1001",
				wantRoles:      []string{"TW_TEACHER"},
				wantSubject:    "EP000001",
				wantSchoolCode: "1001",
			},
			{
				name:           "splits a comma-separated role list",
				subject:        "EP000002",
				schoolCode:     "2002",
				roles:          "TW_TEACHER, TW_SCHOOL_ADMIN ",
				wantRoles:      []string{"TW_TEACHER", "TW_SCHOOL_ADMIN"},
				wantSubject:    "EP000002",
				wantSchoolCode: "2002",
			},
			{
				// Still signs, so PG returns the 401 rather than a 500 here.
				name:           "signs an empty identity when the stub is unset",
				wantRoles:      []string{"TW_TEACHER"},
				wantSubject:    "",
				wantSchoolCode: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Setenv(envPGStubSubject, tt.subject)
				t.Setenv(envPGStubSchoolCode, tt.schoolCode)
				t.Setenv(envPGStubRoles, tt.roles)

				req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
				req.SetPathValue("app", "posts")
				rec := httptest.NewRecorder()

				h.proxy(rec, req)

				if want, got := http.StatusOK, rec.Code; want != got {
					t.Fatalf("want: %d; got: %d", want, got)
				}
				token, ok := strings.CutPrefix(receivedReqHeaders.Get("Authorization"), "Bearer ")
				if !ok {
					t.Fatalf("want: not empty; got: %q", receivedReqHeaders.Get("Authorization"))
				}

				var claims pgClaims
				if _, err := jwt.ParseWithClaims(token, &claims,
					func(*jwt.Token) (any, error) { return []byte(signingKey), nil },
					jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
				); err != nil {
					t.Fatalf("want err: nil; got: %v", err)
				}

				if want, got := tt.wantSubject, claims.Subject; want != got {
					t.Errorf("subject: want %q; got %q", want, got)
				}
				if want, got := tt.wantSchoolCode, claims.SchoolCode; want != got {
					t.Errorf("school_code: want %q; got %q", want, got)
				}
				if want, got := tt.wantRoles, claims.Roles; !reflect.DeepEqual(want, got) {
					t.Errorf("roles: want %v; got %v", want, got)
				}
				// PG rejects a token without these: -4017 for a missing
				// effective_role, -4036 without ATTR_PG_USER.
				if claims.EffectiveRole == "" {
					t.Error("effective_role: want non-empty; got empty")
				}
				if want, got := []string{"ATTR_PG_USER"}, claims.Attributes; !reflect.DeepEqual(want, got) {
					t.Errorf("attributes: want %v; got %v", want, got)
				}
			})
		}
	})
}

func TestProxyErrorHandler(t *testing.T) {
	t.Run("answers 502", func(t *testing.T) {
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		// ReverseProxy calls the error handler with the outbound request: its
		// RequestURI is still what the client asked for, while its URL has been
		// rewritten to the backend.
		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		backendURL, err := url.Parse("http://backend.internal:8080/hello")
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		req.URL = backendURL
		rec := httptest.NewRecorder()

		proxyErrorHandler(rec, req, errors.New("connection refused"))

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("logs the failure", func(t *testing.T) {
		type logEntry struct {
			Level   string `json:"level"`
			Msg     string `json:"msg"`
			Method  string `json:"method"`
			Path    string `json:"path"`
			Backend string `json:"backend"`
			Err     string `json:"err"`
		}

		var logs bytes.Buffer

		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		req := httptest.NewRequest(http.MethodPost, "/api/posts/hello", nil)
		backendURL, err := url.Parse("http://backend.internal:8080/hello")
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		req.URL = backendURL
		rec := httptest.NewRecorder()

		proxyErrorHandler(rec, req, errors.New("connection refused"))

		var entry logEntry
		if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		if want, got := "ERROR", entry.Level; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "failed to proxy request", entry.Msg; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := http.MethodPost, entry.Method; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		// The path the client asked for, not the prefix-stripped one.
		if want, got := "/api/posts/hello", entry.Path; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		// The rewritten backend URL, not the one the client asked for.
		if want, got := "http://backend.internal:8080/hello", entry.Backend; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "connection refused", entry.Err; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("keeps the query string out of the logs", func(t *testing.T) {
		type logEntry struct {
			Path    string `json:"path"`
			Backend string `json:"backend"`
		}

		var logs bytes.Buffer

		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello?token=secret", nil)
		backendURL, err := url.Parse("http://backend.internal:8080/hello?token=secret")
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		req.URL = backendURL
		rec := httptest.NewRecorder()

		proxyErrorHandler(rec, req, errors.New("connection refused"))

		if got := logs.String(); strings.Contains(got, "secret") {
			t.Errorf("want logs: without %q; got: %q", "secret", got)
		}

		var entry logEntry
		if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		if want, got := "/api/posts/hello", entry.Path; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "http://backend.internal:8080/hello", entry.Backend; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}
