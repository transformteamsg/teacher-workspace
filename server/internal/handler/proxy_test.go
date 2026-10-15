package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
	"github.com/transformteamsg/teacher-workspace/server/internal/session"
)

// newRemoteBackend starts a fake remote backend served by handler and returns
// its URL.
func newRemoteBackend(t *testing.T, handler http.Handler) *url.URL {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	return serverURL
}

func TestHandler_proxy(t *testing.T) {
	t.Run("forwards request to the remote backend without the /api/<app> prefix", func(t *testing.T) {
		postsBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "posts:"+r.URL.RequestURI())
		}))
		studentInsightsBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "student-insights:"+r.URL.RequestURI())
		}))

		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    postsBackendURL,
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",

					StudentInsightsManifestURL:       &url.URL{Scheme: "https", Host: "student-insights.example.com", Path: "/mf-manifest.json"},
					StudentInsightsBackendBaseURL:    studentInsightsBackendURL,
					StudentInsightsBackendSigningKey: "student-insights-string-secret-at-least-256-bits-long",
				},
			},
		}

		tests := []struct {
			name   string
			target string
			want   string
		}{
			{
				name:   "posts",
				target: "/api/posts/hello",
				want:   "posts:/hello",
			},
			{
				name:   "student insights",
				target: "/api/student-insights/hello",
				want:   "student-insights:/hello",
			},
			{
				name:   "nested path",
				target: "/api/posts/2026/08/hello",
				want:   "posts:/2026/08/hello",
			},
			{
				name:   "app root",
				target: "/api/posts/",
				want:   "posts:/",
			},
			{
				name:   "escaped path segment",
				target: "/api/posts/a%2Fb",
				want:   "posts:/a%2Fb",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, newSignedInSession())

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, test.target, nil)
				rec := httptest.NewRecorder()

				h.proxy().ServeHTTP(rec, req)

				if want, got := http.StatusOK, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := test.want, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("responds with 404 when the remote app is unknown or not registered", func(t *testing.T) {
		var calls int
		postsBackendURL := newRemoteBackend(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			calls++
		}))

		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    postsBackendURL,
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",
				},
			},
		}

		tests := []struct {
			name   string
			target string
		}{
			{
				name:   "unknown app",
				target: "/api/unknown/hello",
			},
			{
				name:   "unregistered app",
				target: "/api/student-insights/hello",
			},
			{
				name:   "missing path after app",
				target: "/api/posts",
			},
			{
				name:   "missing app",
				target: "/api/",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, newSignedInSession())

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, test.target, nil)
				rec := httptest.NewRecorder()

				h.proxy().ServeHTTP(rec, req)

				if want, got := http.StatusNotFound, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}

				var body struct {
					Message string `json:"message"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if want, got := http.StatusText(http.StatusNotFound), body.Message; want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			})
		}

		if want := 0; want != calls {
			t.Errorf("want: %d; got: %d", want, calls)
		}
	})

	t.Run("responds with 500 when the request has no session", func(t *testing.T) {
		var calls int
		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL: &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL: newRemoteBackend(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						calls++
					})),
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",
				},
			},
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.proxy().ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}

		var body struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := http.StatusText(http.StatusInternalServerError), body.Message; want != got {
			t.Errorf("want body.Message: %q; got: %q", want, got)
		}
		if want := 0; want != calls {
			t.Errorf("want calls: %d; got: %d", want, calls)
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    newRemoteBackend(t, http.NotFoundHandler()),
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",
				},
			},
		}

		var logs bytes.Buffer
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.proxy().ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "no session found in context", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
	})

	t.Run("responds with 500 when the session has no user", func(t *testing.T) {
		var calls int
		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL: &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL: newRemoteBackend(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						calls++
					})),
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",
				},
			},
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.proxy().ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}

		var body struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := http.StatusText(http.StatusInternalServerError), body.Message; want != got {
			t.Errorf("want body.Message: %q; got: %q", want, got)
		}
		if want := 0; want != calls {
			t.Errorf("want calls: %d; got: %d", want, calls)
		}
	})

	t.Run("logs an error when the session has no user", func(t *testing.T) {
		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    newRemoteBackend(t, http.NotFoundHandler()),
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",
				},
			},
		}

		var logs bytes.Buffer
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.proxy().ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "no user found in session", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
	})

	t.Run("attaches a signed JWT for the remote app", func(t *testing.T) {
		echoAuthorization := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, r.Header.Get("Authorization"))
		})

		// The JWT parser returns claim times in time.Local, so now is built in it
		// too for the claims to compare deeply equal.
		now := time.Date(2026, time.September, 29, 10, 0, 0, 0, time.Local)
		ttl := 2 * time.Minute
		h := &Handler{
			now: func() time.Time { return now },
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: ttl,

					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    newRemoteBackend(t, echoAuthorization),
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",

					StudentInsightsManifestURL:       &url.URL{Scheme: "https", Host: "student-insights.example.com", Path: "/mf-manifest.json"},
					StudentInsightsBackendBaseURL:    newRemoteBackend(t, echoAuthorization),
					StudentInsightsBackendSigningKey: "student-insights-string-secret-at-least-256-bits-long",
				},
			},
		}

		tests := []struct {
			name         string
			target       string
			signingKey   string
			wantAudience jwt.ClaimStrings
		}{
			{
				name:         "posts",
				target:       "/api/posts/hello",
				signingKey:   "posts-string-secret-at-least-256-bits-long",
				wantAudience: jwt.ClaimStrings{"pg"},
			},
			{
				name:         "student insights",
				target:       "/api/student-insights/hello",
				signingKey:   "student-insights-string-secret-at-least-256-bits-long",
				wantAudience: jwt.ClaimStrings{"si"},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, newSignedInSession())

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, test.target, nil)
				rec := httptest.NewRecorder()

				h.proxy().ServeHTTP(rec, req)

				if want, got := http.StatusOK, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}

				token, ok := strings.CutPrefix(rec.Body.String(), "Bearer ")
				if !ok {
					t.Fatalf("want: containing %q; got: %q", "Bearer ", rec.Body.String())
				}

				var claims jwt.RegisteredClaims
				if _, err := jwt.ParseWithClaims(token, &claims,
					func(*jwt.Token) (any, error) {
						return []byte(test.signingKey), nil
					},
					jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
					jwt.WithTimeFunc(func() time.Time { return now }),
				); err != nil {
					t.Fatalf("want err: nil; got: %v", err)
				}

				want := jwt.RegisteredClaims{
					Issuer:    "tw",
					Audience:  test.wantAudience,
					IssuedAt:  jwt.NewNumericDate(now),
					ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
				}
				if !reflect.DeepEqual(want, claims) {
					t.Errorf("want: %+v; got: %+v", want, claims)
				}
			})
		}
	})

	t.Run("carries empty role claims in the JWT for a user stored without a role or attributes", func(t *testing.T) {
		const signingKey = "posts-string-secret-at-least-256-bits-long"
		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL: &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL: newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, r.Header.Get("Authorization"))
					})),
					PostsBackendSigningKey: signingKey,
				},
			},
		}

		// A session saved before roles and attributes were stored loads its user
		// with neither.
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.proxy().ServeHTTP(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Fatalf("want rec.Code: %d; got: %d", want, got)
		}

		claims := parseSignedTokenClaims(t, rec.Body.String(), signingKey)

		if got := claims.Role; got != "" {
			t.Errorf("want claims.Role: empty; got: %q", got)
		}
		if got := claims.EffectiveRole; got != "" {
			t.Errorf("want claims.EffectiveRole: empty; got: %q", got)
		}
		// %#v tells [] from nil, which would encode as null.
		if want, got := []string{}, claims.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want claims.Attributes: %#v; got: %#v", want, got)
		}
	})

	t.Run("carries the signed-in user's stored role and attributes in the JWT", func(t *testing.T) {
		const signingKey = "posts-string-secret-at-least-256-bits-long"
		h := &Handler{
			cfg: &config.Config{
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL: time.Minute,

					PostsManifestURL: &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL: newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.WriteString(w, r.Header.Get("Authorization"))
					})),
					PostsBackendSigningKey: signingKey,
				},
			},
		}

		// Unsorted, so a sort would show.
		sess := session.New()
		sess.SetUser(session.User{
			Email:      "teacher@example.com",
			Role:       "ROLE_TEACHER",
			Attributes: []string{"ATTR_SLD", "ATTR_CCE"},
		})

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.proxy().ServeHTTP(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Fatalf("want rec.Code: %d; got: %d", want, got)
		}

		claims := parseSignedTokenClaims(t, rec.Body.String(), signingKey)

		if want, got := "ROLE_TEACHER", claims.Role; want != got {
			t.Errorf("want claims.Role: %q; got: %q", want, got)
		}
		if want, got := "ROLE_TEACHER", claims.EffectiveRole; want != got {
			t.Errorf("want claims.EffectiveRole: %q; got: %q", want, got)
		}
		if want, got := []string{"ATTR_SLD", "ATTR_CCE"}, claims.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want claims.Attributes: %q; got: %q", want, got)
		}
	})
}

func TestNewRemoteBackendProxy(t *testing.T) {
	t.Run("proxies request to the remote backend", func(t *testing.T) {
		remoteBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprintf(w, "%s %s %s", r.Method, r.URL.RequestURI(), body)
		}))

		req := httptest.NewRequest(http.MethodPost, "/search?q=hello&page=2", strings.NewReader(`{"title":"Hello"}`))
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := `POST /search?q=hello&page=2 {"title":"Hello"}`, rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("prepends the base URL path", func(t *testing.T) {
		remoteBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, r.URL.RequestURI())
		}))
		remoteBackendURL.Path = "/v1"

		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if want, got := "/v1/hello", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("sets Authorization from the signed token in context", func(t *testing.T) {
		remoteBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, strings.Join(r.Header.Values("Authorization"), ","))
		}))

		ctx := context.WithValue(t.Context(), ctxKeySignedToken{}, "test-token")

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/hello", nil)
		req.Header.Set("Authorization", "Bearer forged")
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if want, got := "Bearer test-token", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("strips cookies from the forwarded request", func(t *testing.T) {
		remoteBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, strings.Join(r.Header.Values("Cookie"), ","))
		}))

		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		req.AddCookie(&http.Cookie{Name: "session-name", Value: "session-value"})
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if got := rec.Body.String(); got != "" {
			t.Errorf("want: empty; got: %q", got)
		}
	})

	t.Run("strips Set-Cookie from the remote backend response", func(t *testing.T) {
		remoteBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "backend-name", Value: "backend-value"})
			w.Header().Set("X-Custom", "kept")
			_, _ = io.WriteString(w, "Hello world!")
		}))

		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
			t.Errorf("want: empty; got: %q", got)
		}
		if want, got := "kept", rec.Header().Get("X-Custom"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "Hello world!", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("responds with 502 and logs error when the remote backend is unreachable", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		remoteBackendURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		server.Close()
		remoteBackendURL.Path = "/v1"

		var buf bytes.Buffer
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)))

		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/hello", nil)
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}

		var body struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := http.StatusText(http.StatusBadGateway), body.Message; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}

		var record struct {
			Level            string `json:"level"`
			Msg              string `json:"msg"`
			Method           string `json:"method"`
			Path             string `json:"path"`
			RemoteBackendURL string `json:"remote_backend_url"`
			Err              string `json:"err"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := "ERROR", record.Level; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "failed to proxy request", record.Msg; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := http.MethodPost, record.Method; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "/hello", record.Path; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := remoteBackendURL.String()+"/hello", record.RemoteBackendURL; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if record.Err == "" {
			t.Error("want err: non-empty; got: empty")
		}
	})

	t.Run("keeps the query string out of the logs", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		remoteBackendURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		server.Close()

		var buf bytes.Buffer
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/hello?token=secret", nil)
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if got := buf.String(); strings.Contains(got, "secret") {
			t.Errorf("want: not containing %q; got: %q", "secret", got)
		}

		var record struct {
			Path             string `json:"path"`
			RemoteBackendURL string `json:"remote_backend_url"`
		}
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if want, got := "/hello", record.Path; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := remoteBackendURL.String()+"/hello", record.RemoteBackendURL; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("writes no response and logs nothing when the client cancels the request", func(t *testing.T) {
		remoteBackendURL := newRemoteBackend(t, http.NotFoundHandler())

		var buf bytes.Buffer
		ctx, cancel := context.WithCancel(middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil))))
		cancel()

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/hello", nil)
		rec := httptest.NewRecorder()

		newRemoteBackendProxy(remoteBackendURL).ServeHTTP(rec, req)

		if got := rec.Body.String(); got != "" {
			t.Errorf("want: empty; got: %q", got)
		}
		if got := buf.String(); got != "" {
			t.Errorf("want: empty; got: %q", got)
		}
	})
}

// signedTokenClaims names the claims as remote backends read them, so renaming
// one in proxy.go fails the tests.
type signedTokenClaims struct {
	Role          string   `json:"role"`
	EffectiveRole string   `json:"effective_role"`
	Attributes    []string `json:"attributes"`
	jwt.RegisteredClaims
}

// parseSignedTokenClaims verifies authorization as a bearer JWT signed with
// signingKey, and returns its claims.
func parseSignedTokenClaims(t *testing.T, authorization, signingKey string) signedTokenClaims {
	t.Helper()

	token, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok {
		t.Fatalf("want Authorization: containing %q; got: %q", "Bearer ", authorization)
	}

	var claims signedTokenClaims
	if _, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) {
			return []byte(signingKey), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	); err != nil {
		t.Fatalf("jwt.ParseWithClaims: %v", err)
	}

	return claims
}

// newSignedInSession returns a session with a signed-in user, for tests that
// only need the proxy to accept the request.
func newSignedInSession() *session.Session {
	sess := session.New()
	sess.SetUser(session.User{Email: "teacher@example.com", Role: "ROLE_TEACHER"})
	return sess
}
