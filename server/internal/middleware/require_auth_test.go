package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/transformteamsg/teacher-workspace/server/internal/session"
)

func TestRequireAuth(t *testing.T) {
	t.Run("serves the request with the protected handler when the session is authenticated", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		var fallbackCalled bool
		fallback := func(w http.ResponseWriter, r *http.Request) {
			fallbackCalled = true
		}

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequireAuth(fallback)(next).ServeHTTP(rec, req)

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if fallbackCalled {
			t.Error("want fallbackCalled: false; got: true")
		}
	})

	t.Run("serves the request with the fallback handler when the session is not authenticated", func(t *testing.T) {
		sess := session.New()

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		fallback := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequireAuth(fallback)(next).ServeHTTP(rec, req)

		if want, got := http.StatusUnauthorized, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("aborts with a 500 when the request has no session", func(t *testing.T) {
		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		var fallbackCalled bool
		fallback := func(w http.ResponseWriter, r *http.Request) {
			fallbackCalled = true
		}

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequireAuth(fallback)(next).ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
		if fallbackCalled {
			t.Error("want fallbackCalled: false; got: true")
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		fallback := func(w http.ResponseWriter, r *http.Request) {}

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequireAuth(fallback)(next).ServeHTTP(rec, req)

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
}
