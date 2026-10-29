package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/transformteamsg/teacher-workspace/server/internal/session"
)

func TestCSRF(t *testing.T) {
	t.Run("serves a safe method without a token", func(t *testing.T) {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, "QUERY"} {
			t.Run(method, func(t *testing.T) {
				sess := session.New()
				sess.SetUser(session.User{Email: "teacher@example.com"})

				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
				})

				ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = WithSession(ctx, sess)

				req := httptest.NewRequest(method, "/", nil).WithContext(ctx)
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				rec := httptest.NewRecorder()

				CSRF(writeForbidden)(next).ServeHTTP(rec, req)

				if want, got := http.StatusCreated, rec.Code; want != got {
					t.Errorf("want rec.Code: %d; got: %d", want, got)
				}
			})
		}
	})

	t.Run("serves a same-origin request with a valid token", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			t.Run(method, func(t *testing.T) {
				sess := session.New()
				sess.SetUser(session.User{Email: "teacher@example.com"})

				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
				})

				ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = WithSession(ctx, sess)

				req := httptest.NewRequest(method, "/", nil).WithContext(ctx)
				req.Header.Set("Sec-Fetch-Site", "same-origin")
				req.Header.Set(CSRFTokenHeader, sess.CSRFToken())
				rec := httptest.NewRecorder()

				CSRF(writeForbidden)(next).ServeHTTP(rec, req)

				if want, got := http.StatusCreated, rec.Code; want != got {
					t.Errorf("want rec.Code: %d; got: %d", want, got)
				}
			})
		}
	})

	t.Run("rejects a cross-origin request with a valid token", func(t *testing.T) {
		for _, site := range []string{"cross-site", "same-site"} {
			t.Run(site, func(t *testing.T) {
				sess := session.New()
				sess.SetUser(session.User{Email: "teacher@example.com"})

				var nextCalled bool
				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					nextCalled = true
				})

				ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = WithSession(ctx, sess)

				req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
				req.Header.Set("Sec-Fetch-Site", site)
				req.Header.Set(CSRFTokenHeader, sess.CSRFToken())
				rec := httptest.NewRecorder()

				CSRF(writeForbidden)(next).ServeHTTP(rec, req)

				if want, got := http.StatusForbidden, rec.Code; want != got {
					t.Errorf("want rec.Code: %d; got: %d", want, got)
				}
				if nextCalled {
					t.Error("want nextCalled: false; got: true")
				}
			})
		}
	})

	t.Run("rejects a request whose Origin names another host when Sec-Fetch-Site is missing", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "http://example.com/", nil).WithContext(ctx)
		req.Header.Set("Origin", "http://attacker.example")
		req.Header.Set(CSRFTokenHeader, sess.CSRFToken())
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("serves a request whose Origin names its host when Sec-Fetch-Site is missing", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "http://example.com/", nil).WithContext(ctx)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set(CSRFTokenHeader, sess.CSRFToken())
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
	})

	t.Run("serves a request with neither Sec-Fetch-Site nor Origin when it has a valid token", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set(CSRFTokenHeader, sess.CSRFToken())
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
	})

	t.Run("rejects a request with neither Sec-Fetch-Site nor Origin when it has no token", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("rejects a same-origin request without a token", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("rejects a same-origin request with a token that does not verify against the session", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set(CSRFTokenHeader, session.New().CSRFToken())
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("rejects a same-origin request with a token issued before the user was set", func(t *testing.T) {
		sess := session.New()
		token := sess.CSRFToken()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set(CSRFTokenHeader, token)
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("logs a warning when it rejects a cross-origin request", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set(CSRFTokenHeader, sess.CSRFToken())
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelWarn.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "rejected cross-origin request", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "Sec-Fetch-Site", records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})

	t.Run("logs a warning when it rejects a same-origin request without a valid token", func(t *testing.T) {
		sess := session.New()
		sess.SetUser(session.User{Email: "teacher@example.com"})

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)
		ctx = WithSession(ctx, sess)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelWarn.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "rejected request without a valid CSRF token", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
	})

	t.Run("aborts with a 500 when the request has no session", func(t *testing.T) {
		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if nextCalled {
			t.Error("want nextCalled: false; got: true")
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		CSRF(writeForbidden)(next).ServeHTTP(rec, req)

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

// writeForbidden stands in for the reject handler, responding with 403.
func writeForbidden(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusForbidden)
}
