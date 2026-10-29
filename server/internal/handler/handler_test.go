package handler

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
	"github.com/transformteamsg/teacher-workspace/server/internal/session"
	"github.com/transformteamsg/teacher-workspace/server/internal/session/memstore"
)

func TestHandler_Routes(t *testing.T) {
	t.Run("signs the user out on a sign-out request without a CSRF token", func(t *testing.T) {
		h := &Handler{
			cfg: &config.Config{
				Session: config.SessionConfig{
					Name:             "tw_session",
					DefaultTTL:       time.Hour,
					AuthenticatedTTL: time.Hour,
				},
			},
		}

		store := memstore.New()
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})
		if err := session.Commit(t.Context(), store, sess, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/logout", nil)
		req.AddCookie(&http.Cookie{Name: "tw_session", Value: sess.ID()})
		rec := httptest.NewRecorder()

		h.Routes(store).ServeHTTP(rec, req)

		if want, got := http.StatusSeeOther, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}
		loaded, err := session.Load(t.Context(), store, sess.ID())
		if err != nil {
			t.Fatalf("session.Load: %v", err)
		}
		if loaded.IsAuthenticated() {
			t.Error("want loaded.IsAuthenticated(): false; got: true")
		}
	})

	t.Run("rejects a cross-origin state-changing request under /api/ before it reaches the remote backend", func(t *testing.T) {
		var reached atomic.Bool
		postsBackendURL := newRemoteBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached.Store(true)
		}))

		h := &Handler{
			cfg: &config.Config{
				Session: config.SessionConfig{
					Name:             "tw_session",
					DefaultTTL:       time.Hour,
					AuthenticatedTTL: time.Hour,
				},
				RemoteApps: config.RemoteAppsConfig{
					SignedTokenTTL:         time.Minute,
					PostsManifestURL:       &url.URL{Scheme: "https", Host: "posts.example.com", Path: "/mf-manifest.json"},
					PostsBackendBaseURL:    postsBackendURL,
					PostsBackendSigningKey: "posts-string-secret-at-least-256-bits-long",
				},
			},
		}

		store := memstore.New()
		sess := session.New()
		sess.SetUser(session.User{
			ID:         "staff-1",
			Email:      "teacher@example.com",
			Role:       "ROLE_TEACHER",
			SchoolCode: "1234",
			Attributes: []string{},
		})
		token := sess.CSRFToken()
		if err := session.Commit(t.Context(), store, sess, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/posts/hello", nil)
		req.AddCookie(&http.Cookie{Name: "tw_session", Value: sess.ID()})
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set(middleware.CSRFTokenHeader, token)
		rec := httptest.NewRecorder()

		h.Routes(store).ServeHTTP(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}
		if reached.Load() {
			t.Error("want reached: false; got: true")
		}
	})
}
