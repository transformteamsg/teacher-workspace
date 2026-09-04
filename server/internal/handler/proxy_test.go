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

		h := New(&cfg, nil)

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

		h := New(&cfg, nil)

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

		h := New(&cfg, nil)

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

		h := New(&cfg, nil)

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

		h := New(&cfg, nil)

		currentTime := time.Now()
		tests := []struct {
			app        string
			signingKey string
			wantClaims jwt.RegisteredClaims
		}{
			{
				app:        "student-insights",
				signingKey: "student-insights-string-secret-at-least-256-bits-long",
				wantClaims: jwt.RegisteredClaims{
					Issuer:    "TW",
					Audience:  jwt.ClaimStrings{"si"},
					IssuedAt:  jwt.NewNumericDate(currentTime),
					ExpiresAt: jwt.NewNumericDate(currentTime.Add(ttl)),
				},
			},
			{
				app:        "posts",
				signingKey: "posts-string-secret-at-least-256-bits-long",
				wantClaims: jwt.RegisteredClaims{
					Issuer:    "TW",
					Audience:  jwt.ClaimStrings{"pg"},
					IssuedAt:  jwt.NewNumericDate(currentTime),
					ExpiresAt: jwt.NewNumericDate(currentTime.Add(ttl)),
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

			var claims jwt.RegisteredClaims
			if _, err := jwt.ParseWithClaims(token, &claims,
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
