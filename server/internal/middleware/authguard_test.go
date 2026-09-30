package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

func TestRequireAuth(t *testing.T) {
	authenticatedSession := func() *session.Session {
		sess := session.New()
		sess.SetUser(&session.User{Email: "teacher@example.com"})
		return sess
	}

	tests := []struct {
		name         string
		method       string
		target       string
		sess         *session.Session
		wantNext     bool
		wantCode     int
		wantLocation string
		wantBody     string
	}{
		{
			name:     "passes through to /auth/edupass without authentication",
			method:   http.MethodGet,
			target:   "/auth/edupass",
			sess:     session.New(),
			wantNext: true,
		},
		{
			name:     "passes through to /auth/edupass/callback without authentication",
			method:   http.MethodGet,
			target:   "/auth/edupass/callback",
			sess:     session.New(),
			wantNext: true,
		},
		{
			name:     "passes through to /auth/edupass with authentication",
			method:   http.MethodGet,
			target:   "/auth/edupass",
			sess:     authenticatedSession(),
			wantNext: true,
		},
		{
			name:     "passes through to /login without authentication",
			method:   http.MethodGet,
			target:   "/login",
			sess:     session.New(),
			wantNext: true,
		},
		{
			name:         "redirects authenticated user at /login to /",
			method:       http.MethodGet,
			target:       "/login",
			sess:         authenticatedSession(),
			wantCode:     http.StatusFound,
			wantLocation: "/",
		},
		{
			name:         "redirects unauthenticated browser request to /login with return_to",
			method:       http.MethodGet,
			target:       "/posts/123",
			sess:         session.New(),
			wantCode:     http.StatusFound,
			wantLocation: "/login?return_to=%2Fposts%2F123",
		},
		{
			name:         "redirects unauthenticated root request to /login without return_to",
			method:       http.MethodGet,
			target:       "/",
			sess:         session.New(),
			wantCode:     http.StatusFound,
			wantLocation: "/login",
		},
		{
			name:         "preserves query string in return_to",
			method:       http.MethodGet,
			target:       "/posts?tab=drafts",
			sess:         session.New(),
			wantCode:     http.StatusFound,
			wantLocation: "/login?return_to=%2Fposts%3Ftab%3Ddrafts",
		},
		{
			name:     "returns 401 JSON for unauthenticated API request",
			method:   http.MethodGet,
			target:   "/api/posts/hello",
			sess:     session.New(),
			wantCode: http.StatusUnauthorized,
			wantBody: "{\"message\":\"Unauthorized\"}\n",
		},
		{
			name:     "returns 401 JSON for unauthenticated /api/ root",
			method:   http.MethodGet,
			target:   "/api/",
			sess:     session.New(),
			wantCode: http.StatusUnauthorized,
			wantBody: "{\"message\":\"Unauthorized\"}\n",
		},
		{
			name:     "passes through authenticated request to a protected page",
			method:   http.MethodGet,
			target:   "/posts/123",
			sess:     authenticatedSession(),
			wantNext: true,
		},
		{
			name:     "passes through authenticated request to an API endpoint",
			method:   http.MethodGet,
			target:   "/api/posts/hello",
			sess:     authenticatedSession(),
			wantNext: true,
		},
		{
			name:         "redirects when session is missing from context",
			method:       http.MethodGet,
			target:       "/posts/123",
			sess:         nil,
			wantCode:     http.StatusFound,
			wantLocation: "/login?return_to=%2Fposts%2F123",
		},
		{
			name:     "returns 401 when session is missing from context for API request",
			method:   http.MethodGet,
			target:   "/api/posts/hello",
			sess:     nil,
			wantCode: http.StatusUnauthorized,
			wantBody: "{\"message\":\"Unauthorized\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nextCalled bool
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				nextCalled = true
			})

			req := httptest.NewRequest(tt.method, tt.target, nil)
			if tt.sess != nil {
				req = req.WithContext(WithSession(req.Context(), tt.sess))
			}
			rec := httptest.NewRecorder()

			RequireAuth()(next).ServeHTTP(rec, req)

			if tt.wantNext {
				if !nextCalled {
					t.Error("want next handler to be called")
				}
				return
			}

			if nextCalled {
				t.Error("want next handler not to be called")
			}
			if want, got := tt.wantCode, rec.Code; want != got {
				t.Errorf("want: %d; got: %d", want, got)
			}
			if tt.wantLocation != "" {
				if want, got := tt.wantLocation, rec.Header().Get("Location"); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			}
			if tt.wantBody != "" {
				if want, got := tt.wantBody, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			}
		})
	}
}
