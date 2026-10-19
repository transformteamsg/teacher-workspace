package middleware

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/transformteamsg/teacher-workspace/server/internal/session"
	"github.com/transformteamsg/teacher-workspace/server/internal/session/memstore"
)

func TestSession(t *testing.T) {
	t.Run("loads the session identified by the session cookie", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		seeded.SetUser(session.User{Email: "teacher@example.com"})
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		if want, got := seeded.ID(), loaded.ID(); want != got {
			t.Errorf("want loaded.ID(): %q; got: %q", want, got)
		}
		user, ok := loaded.User()
		if !ok {
			t.Fatal("want loaded.User() ok: true; got: false")
		}
		if want, got := "teacher@example.com", user.Email; want != got {
			t.Errorf("want user.Email: %q; got: %q", want, got)
		}
	})

	t.Run("creates a new session when the session cookie is missing", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
	})

	t.Run("creates a new session when the session cookie names no stored session", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: "id-1",
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		if want, got := "id-1", loaded.ID(); want == got {
			t.Errorf("want loaded.ID(): != %q; got: %q", want, got)
		}
	})

	t.Run("creates a new session when the session cookie names a session that cannot be decoded", func(t *testing.T) {
		store := newFakeStore(t, entry{
			id:   "id-1",
			data: "not json",
		})

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: "id-1",
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		if want, got := "id-1", loaded.ID(); want == got {
			t.Errorf("want loaded.ID(): != %q; got: %q", want, got)
		}
	})

	t.Run("logs a warning when the session cookie names a session that cannot be decoded", func(t *testing.T) {
		store := newFakeStore(t, entry{
			id:   "id-1",
			data: "not json",
		})

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: "id-1",
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelWarn.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "discarding undecodable session", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := session.ErrUndecodable.Error(), records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})

	t.Run("aborts with a 500 when the store fails to load the session", func(t *testing.T) {
		store := newFakeStore(t, entry{
			id:   "id-1",
			data: "session data",
		})
		store.getErr = errors.New("store unavailable")

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var called bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: "id-1",
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if called {
			t.Error("want called: false; got: true")
		}
		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
			t.Errorf("want Set-Cookie: empty; got: %q", got)
		}
		data, err := store.Store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("memstore.Store.Get: %v", err)
		}
		if want, got := "session data", string(data); want != got {
			t.Errorf("want string(data): %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the store fails to load the session", func(t *testing.T) {
		store := newFakeStore(t)
		store.getErr = errors.New("store unavailable")

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: "id-1",
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to load session", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "store unavailable", records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})

	t.Run("does nothing when the client disconnects before the session loads", func(t *testing.T) {
		store := newFakeStore(t, entry{
			id:   "id-1",
			data: "session data",
		})

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var called bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: "id-1",
		})
		rec := &statusRecordingWriter{ResponseRecorder: httptest.NewRecorder()}

		Session(store, opts)(next).ServeHTTP(rec, req)

		if called {
			t.Error("want called: false; got: true")
		}
		if got := rec.statuses; len(got) != 0 {
			t.Errorf("want rec.statuses: empty; got: %v", got)
		}
		if got := rec.Body.String(); got != "" {
			t.Errorf("want rec.Body: empty; got: %q", got)
		}
		if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
			t.Errorf("want Set-Cookie: empty; got: %q", got)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "session data", string(data); want != got {
			t.Errorf("want string(data): %q; got: %q", want, got)
		}
		if got := logs.String(); got != "" {
			t.Errorf("want logs: empty; got: %q", got)
		}
	})

	t.Run("sets the session to expire after the default TTL when no user is attached", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		ttl, ok := store.ttls[loaded.ID()]
		if !ok {
			t.Fatal("want store.ttls[loaded.ID()] ok: true; got: false")
		}
		if want := opts.DefaultTTL; want != ttl {
			t.Errorf("want ttl: %v; got: %v", want, ttl)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := 3600, cookies[0].MaxAge; want != got {
			t.Errorf("want cookies[0].MaxAge: %d; got: %d", want, got)
		}
	})

	t.Run("sets the session to expire after the authenticated TTL when a user is attached", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ok bool
			loaded, ok = SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			loaded.SetUser(session.User{Email: "teacher@example.com"})
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		ttl, ok := store.ttls[loaded.ID()]
		if !ok {
			t.Fatal("want store.ttls[loaded.ID()] ok: true; got: false")
		}
		if want := opts.AuthenticatedTTL; want != ttl {
			t.Errorf("want ttl: %v; got: %v", want, ttl)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := 86400, cookies[0].MaxAge; want != got {
			t.Errorf("want cookies[0].MaxAge: %d; got: %d", want, got)
		}
	})

	t.Run("saves changes made before the handler's first write", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ok bool
			loaded, ok = SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			loaded.Set("theme", "dark")
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		reloaded, err := session.Load(t.Context(), store.Store, loaded.ID())
		if err != nil {
			t.Fatalf("session.Load: %v", err)
		}
		// Load returns a new session when nothing is stored under the ID, so a
		// matching ID confirms the session was saved before its value is checked.
		if want, got := loaded.ID(), reloaded.ID(); want != got {
			t.Fatalf("want reloaded.ID(): %q; got: %q", want, got)
		}
		theme, ok := reloaded.Get[string]("theme")
		if !ok {
			t.Fatal("want reloaded.Get[string](\"theme\") ok: true; got: false")
		}
		if want := "dark"; want != theme {
			t.Errorf("want theme: %q; got: %q", want, theme)
		}
	})

	t.Run("does not save changes made after the handler's first write", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ok bool
			loaded, ok = SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			_, _ = w.Write([]byte("ok"))
			loaded.Set("theme", "dark")
			_, _ = w.Write([]byte(" again"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		reloaded, err := session.Load(t.Context(), store.Store, loaded.ID())
		if err != nil {
			t.Fatalf("session.Load: %v", err)
		}
		// Load returns a new session when nothing is stored under the ID, so a
		// matching ID confirms the session was saved before its value is checked.
		if want, got := loaded.ID(), reloaded.ID(); want != got {
			t.Fatalf("want reloaded.ID(): %q; got: %q", want, got)
		}
		if _, ok := reloaded.Get[string]("theme"); ok {
			t.Error("want reloaded.Get[string](\"theme\") ok: false; got: true")
		}
	})

	t.Run("saves the session when the handler writes nothing", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		data, err := store.Get(t.Context(), loaded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data == nil {
			t.Error("want data: non-nil; got: nil")
		}
	})

	t.Run("saves the session when the client disconnects while the handler runs", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
			cancel()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		data, err := store.Get(t.Context(), loaded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data == nil {
			t.Error("want data: non-nil; got: nil")
		}
	})

	t.Run("replaces the handler's response with a 500 when the session cannot be saved", func(t *testing.T) {
		setRequestIDHeader := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-ID", "req-1")
				next.ServeHTTP(w, r)
			})
		}

		store := newFakeStore(t)
		store.writeErr = errors.New("store unavailable")

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Foo", "bar")
			w.Header().Set("Content-Length", "11")
			_, _ = w.Write([]byte(`{"ok":true}`))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		setRequestIDHeader(Session(store, opts)(next)).ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := http.StatusText(http.StatusInternalServerError), rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
		header := rec.Result().Header
		if got := header.Get("X-Foo"); got != "" {
			t.Errorf("want X-Foo: empty; got: %q", got)
		}
		if got := header.Get("Content-Length"); got != "" {
			t.Errorf("want Content-Length: empty; got: %q", got)
		}
		if got := header.Values("Set-Cookie"); len(got) != 0 {
			t.Errorf("want Set-Cookie: empty; got: %q", got)
		}
		if want, got := "req-1", header.Get("X-Request-ID"); want != got {
			t.Errorf("want X-Request-ID: %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the session cannot be saved", func(t *testing.T) {
		store := newFakeStore(t)
		store.writeErr = errors.New("store unavailable")

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to commit session", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "store unavailable", records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})

	t.Run("responds with a 500 when the handler writes nothing and the session cannot be saved", func(t *testing.T) {
		store := newFakeStore(t)
		store.writeErr = errors.New("store unavailable")

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := http.StatusText(http.StatusInternalServerError), rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
	})

	t.Run("sets the session cookie when the handler writes", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := opts.Name, cookies[0].Name; want != got {
			t.Errorf("want cookies[0].Name: %q; got: %q", want, got)
		}
		if want, got := loaded.ID(), cookies[0].Value; want != got {
			t.Errorf("want cookies[0].Value: %q; got: %q", want, got)
		}
		if want, got := "/", cookies[0].Path; want != got {
			t.Errorf("want cookies[0].Path: %q; got: %q", want, got)
		}
		if !cookies[0].HttpOnly {
			t.Error("want cookies[0].HttpOnly: true; got: false")
		}
		if want, got := http.SameSiteLaxMode, cookies[0].SameSite; want != got {
			t.Errorf("want cookies[0].SameSite: %v; got: %v", want, got)
		}
	})

	t.Run("sets the session cookie when the handler writes nothing", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaded, _ = SessionFromContext(r.Context())
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := loaded.ID(), cookies[0].Value; want != got {
			t.Errorf("want cookies[0].Value: %q; got: %q", want, got)
		}
	})

	t.Run("sets the session cookie's Secure attribute", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			secure bool
		}{
			{
				name:   "to true when the Secure option is set",
				secure: true,
			},
			{
				name:   "to false when the Secure option is unset",
				secure: false,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				store := newFakeStore(t)

				opts := SessionOptions{
					Name:             "sid",
					DefaultTTL:       time.Hour,
					AuthenticatedTTL: 24 * time.Hour,
					Secure:           test.secure,
				}

				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("ok"))
				})

				ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				Session(store, opts)(next).ServeHTTP(rec, req)

				cookies := rec.Result().Cookies()
				if len(cookies) == 0 {
					t.Fatal("want cookies: non-empty; got: empty")
				}
				if want, got := test.secure, cookies[0].Secure; want != got {
					t.Errorf("want cookies[0].Secure: %t; got: %t", want, got)
				}
			})
		}
	})

	t.Run("sets Cache-Control to no-store when the handler sets its own", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=60")
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if want, got := []string{"no-store"}, rec.Result().Header.Values("Cache-Control"); !slices.Equal(want, got) {
			t.Errorf("want Cache-Control: %q; got: %q", want, got)
		}
	})

	t.Run("keeps the rest of the handler's response", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Foo", "bar")
			http.SetCookie(w, &http.Cookie{
				Name:  "theme",
				Value: "dark",
			})
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello, "))
			_, _ = w.Write([]byte("world"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "bar", rec.Result().Header.Get("X-Foo"); want != got {
			t.Errorf("want X-Foo: %q; got: %q", want, got)
		}
		cookies := make(map[string]string)
		for _, c := range rec.Result().Cookies() {
			cookies[c.Name] = c.Value
		}
		theme, ok := cookies["theme"]
		if !ok {
			t.Fatal("want cookies[\"theme\"] ok: true; got: false")
		}
		if want := "dark"; want != theme {
			t.Errorf("want theme: %q; got: %q", want, theme)
		}
		if want, got := "hello, world", rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
	})

	t.Run("moves the session to the new ID when its ID changes", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ok bool
			loaded, ok = SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			loaded.SetUser(session.User{Email: "teacher@example.com"})
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		if want, got := seeded.ID(), loaded.ID(); want == got {
			t.Fatalf("want loaded.ID(): != %q; got: %q", want, got)
		}
		newData, err := store.Get(t.Context(), loaded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if newData == nil {
			t.Error("want newData: non-nil; got: nil")
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := loaded.ID(), cookies[0].Value; want != got {
			t.Errorf("want cookies[0].Value: %q; got: %q", want, got)
		}
		oldData, err := store.Get(t.Context(), seeded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if oldData != nil {
			t.Error("want oldData: nil; got: non-nil")
		}
	})

	t.Run("saves the session under the same ID when its ID is unchanged", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			sess.Set("theme", "dark")
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		reloaded, err := session.Load(t.Context(), store.Store, seeded.ID())
		if err != nil {
			t.Fatalf("session.Load: %v", err)
		}
		// Load returns a new session when nothing is stored under the ID, so a
		// matching ID confirms the session was saved before its value is checked.
		if want, got := seeded.ID(), reloaded.ID(); want != got {
			t.Fatalf("want reloaded.ID(): %q; got: %q", want, got)
		}
		theme, ok := reloaded.Get[string]("theme")
		if !ok {
			t.Fatal("want reloaded.Get[string](\"theme\") ok: true; got: false")
		}
		if want := "dark"; want != theme {
			t.Errorf("want theme: %q; got: %q", want, theme)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := seeded.ID(), cookies[0].Value; want != got {
			t.Errorf("want cookies[0].Value: %q; got: %q", want, got)
		}
	})

	t.Run("completes the request when the superseded entry cannot be removed", func(t *testing.T) {
		store := newFakeStore(t)
		store.deleteErr = errors.New("store unavailable")
		seeded := session.New()
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		var loaded *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ok bool
			loaded, ok = SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			loaded.SetUser(session.User{Email: "teacher@example.com"})
			w.WriteHeader(http.StatusNoContent)
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if loaded == nil {
			t.Fatal("want loaded: non-nil; got: nil")
		}
		if want, got := seeded.ID(), loaded.ID(); want == got {
			t.Fatalf("want loaded.ID(): != %q; got: %q", want, got)
		}
		if want, got := http.StatusNoContent, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := loaded.ID(), cookies[0].Value; want != got {
			t.Errorf("want cookies[0].Value: %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the superseded entry cannot be removed", func(t *testing.T) {
		store := newFakeStore(t)
		store.deleteErr = errors.New("store unavailable")
		seeded := session.New()
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			sess.SetUser(session.User{Email: "teacher@example.com"})
			w.WriteHeader(http.StatusNoContent)
		})

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to delete superseded session", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "store unavailable", records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})

	t.Run("removes the session from the store when the handler destroys it", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		seeded.SetUser(session.User{Email: "teacher@example.com"})
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			sess.Destroy()
			w.WriteHeader(http.StatusSeeOther)
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		data, err := store.Get(t.Context(), seeded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("expires the session cookie when the handler destroys the session", func(t *testing.T) {
		store := newFakeStore(t)

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				t.Fatal("want SessionFromContext(r.Context()) ok: true; got: false")
			}
			sess.Destroy()
			w.WriteHeader(http.StatusSeeOther)
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		cookies := rec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("want cookies: non-empty; got: empty")
		}
		if want, got := opts.Name, cookies[0].Name; want != got {
			t.Errorf("want cookies[0].Name: %q; got: %q", want, got)
		}
		if got := cookies[0].Value; got != "" {
			t.Errorf("want cookies[0].Value: empty; got: %q", got)
		}
		if want, got := "/", cookies[0].Path; want != got {
			t.Errorf("want cookies[0].Path: %q; got: %q", want, got)
		}
		if got := cookies[0].MaxAge; got >= 0 {
			t.Errorf("want cookies[0].MaxAge: < 0; got: %d", got)
		}
	})

	t.Run("does not save the session when it is no longer in the store", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		seeded.SetUser(session.User{Email: "teacher@example.com"})
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		// Deleting the entry mid-request stands in for another request
		// destroying the session.
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := store.Delete(r.Context(), seeded.ID()); err != nil {
				t.Fatalf("store.Delete: %v", err)
			}
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		data, err := store.Get(t.Context(), seeded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("leaves the session cookie as it is when the session is no longer in the store", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		seeded.SetUser(session.User{Email: "teacher@example.com"})
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		// Deleting the entry mid-request stands in for another request
		// destroying the session.
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := store.Delete(r.Context(), seeded.ID()); err != nil {
				t.Fatalf("store.Delete: %v", err)
			}
			_, _ = w.Write([]byte("ok"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
			t.Errorf("want Set-Cookie: empty; got: %q", got)
		}
	})

	t.Run("keeps the rest of the handler's response when the session is no longer in the store", func(t *testing.T) {
		store := newFakeStore(t)
		seeded := session.New()
		if err := session.Commit(t.Context(), store.Store, seeded, time.Hour); err != nil {
			t.Fatalf("session.Commit: %v", err)
		}

		opts := SessionOptions{
			Name:             "sid",
			DefaultTTL:       time.Hour,
			AuthenticatedTTL: 24 * time.Hour,
		}

		// Deleting the entry mid-request stands in for another request
		// destroying the session.
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := store.Delete(r.Context(), seeded.ID()); err != nil {
				t.Fatalf("store.Delete: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello, world"))
		})

		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{
			Name:  opts.Name,
			Value: seeded.ID(),
		})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "hello, world", rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
	})
}

func TestSessionFromContext(t *testing.T) {
	t.Run("returns the session attached to the context", func(t *testing.T) {
		sess := session.New()
		ctx := WithSession(t.Context(), sess)

		attached, ok := SessionFromContext(ctx)

		if !ok {
			t.Fatal("want SessionFromContext(ctx) ok: true; got: false")
		}
		if sess != attached {
			t.Errorf("want attached: %p; got: %p", sess, attached)
		}
	})

	t.Run("reports no session when none is attached", func(t *testing.T) {
		_, ok := SessionFromContext(t.Context())

		if ok {
			t.Error("want SessionFromContext(t.Context()) ok: false; got: true")
		}
	})
}

func TestSessionResponseWriter_WriteHeader(t *testing.T) {
	t.Run("does not run finalise before sending a 1xx status other than 101", func(t *testing.T) {
		var finalised bool
		rec := httptest.NewRecorder()
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				finalised = true
				return nil
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		rw.WriteHeader(http.StatusEarlyHints)

		if finalised {
			t.Error("want finalised: false; got: true")
		}
	})

	t.Run("runs finalise before sending", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			status int
		}{
			{
				name:   "a final status",
				status: http.StatusCreated,
			},
			{
				name:   "101 Switching Protocols",
				status: http.StatusSwitchingProtocols,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				rw := &sessionResponseWriter{
					ResponseWriter: rec,
					finalise: func(h http.Header) error {
						h.Add("Set-Cookie", "sid=id-1")
						return nil
					},
					initialHeader: http.Header{},
					logger:        slog.New(slog.DiscardHandler),
				}

				rw.WriteHeader(test.status)

				if want, got := []string{"sid=id-1"}, rec.Result().Header.Values("Set-Cookie"); !slices.Equal(want, got) {
					t.Errorf("want Set-Cookie: %q; got: %q", want, got)
				}
				if want, got := test.status, rec.Code; want != got {
					t.Errorf("want rec.Code: %d; got: %d", want, got)
				}
			})
		}
	})

	t.Run("sends a 500 in place of the status when finalise fails", func(t *testing.T) {
		rec := &statusRecordingWriter{ResponseRecorder: httptest.NewRecorder()}
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				return errors.New("store unavailable")
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		rw.WriteHeader(http.StatusCreated)

		if want, got := []int{http.StatusInternalServerError}, rec.statuses; !slices.Equal(want, got) {
			t.Errorf("want rec.statuses: %v; got: %v", want, got)
		}
	})
}

func TestSessionResponseWriter_Write(t *testing.T) {
	t.Run("runs finalise before writing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				h.Add("Set-Cookie", "sid=id-1")
				return nil
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		n, err := rw.Write([]byte("hello"))

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want := 5; want != n {
			t.Errorf("want n: %d; got: %d", want, n)
		}
		if want, got := []string{"sid=id-1"}, rec.Result().Header.Values("Set-Cookie"); !slices.Equal(want, got) {
			t.Errorf("want Set-Cookie: %q; got: %q", want, got)
		}
		if want, got := "hello", rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the write when finalise fails", func(t *testing.T) {
		finaliseErr := errors.New("store unavailable")
		rec := httptest.NewRecorder()
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				return finaliseErr
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		n, err := rw.Write([]byte("hello"))

		if !errors.Is(err, finaliseErr) {
			t.Errorf("want err: %v; got: %v", finaliseErr, err)
		}
		if want := 0; want != n {
			t.Errorf("want n: %d; got: %d", want, n)
		}
		if want, got := "hello", rec.Body.String(); strings.Contains(got, want) {
			t.Errorf("want rec.Body: not containing %q; got: %q", want, got)
		}
	})
}

func TestSessionResponseWriter_Flush(t *testing.T) {
	t.Run("runs finalise before flushing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				h.Add("Set-Cookie", "sid=id-1")
				return nil
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		rw.Flush()

		if !rec.Flushed {
			t.Error("want rec.Flushed: true; got: false")
		}
		if want, got := []string{"sid=id-1"}, rec.Result().Header.Values("Set-Cookie"); !slices.Equal(want, got) {
			t.Errorf("want Set-Cookie: %q; got: %q", want, got)
		}
	})

	t.Run("returns without flushing when finalise fails", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				return errors.New("store unavailable")
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		rw.Flush()

		if rec.Flushed {
			t.Error("want rec.Flushed: false; got: true")
		}
	})
}

func TestSessionResponseWriter_Hijack(t *testing.T) {
	t.Run("runs finalise before handing over the connection", func(t *testing.T) {
		conn, peer := net.Pipe()
		t.Cleanup(func() {
			_ = conn.Close()
			_ = peer.Close()
		})

		rec := &hijackableWriter{
			ResponseRecorder: httptest.NewRecorder(),
			conn:             conn,
		}
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				h.Add("Set-Cookie", "sid=id-1")
				return nil
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		hijacked, brw, err := rw.Hijack()

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if conn != hijacked {
			t.Errorf("want hijacked: %v; got: %v", conn, hijacked)
		}
		if brw == nil {
			t.Error("want brw: non-nil; got: nil")
		}
		if want, got := []string{"sid=id-1"}, rec.hijackedHeader.Values("Set-Cookie"); !slices.Equal(want, got) {
			t.Errorf("want Set-Cookie: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the hijack when finalise fails", func(t *testing.T) {
		conn, peer := net.Pipe()
		t.Cleanup(func() {
			_ = conn.Close()
			_ = peer.Close()
		})

		finaliseErr := errors.New("store unavailable")
		rec := &hijackableWriter{
			ResponseRecorder: httptest.NewRecorder(),
			conn:             conn,
		}
		rw := &sessionResponseWriter{
			ResponseWriter: rec,
			finalise: func(h http.Header) error {
				return finaliseErr
			},
			initialHeader: http.Header{},
			logger:        slog.New(slog.DiscardHandler),
		}

		hijacked, brw, err := rw.Hijack()

		if !errors.Is(err, finaliseErr) {
			t.Errorf("want err: %v; got: %v", finaliseErr, err)
		}
		if hijacked != nil {
			t.Error("want hijacked: nil; got: non-nil")
		}
		if brw != nil {
			t.Error("want brw: nil; got: non-nil")
		}
		if rec.hijacked {
			t.Error("want rec.hijacked: false; got: true")
		}
	})
}

// Asserts at compile time that *fakeStore satisfies session.Store.
var _ session.Store = (*fakeStore)(nil)

// fakeStore is a session.Store backed by a memstore.Store. It records the TTL
// of each Create and Update call. When getErr is set, Get returns it; when
// writeErr is set, Create and Update return it; when deleteErr is set, Delete
// returns it. None of them touches the entries when returning an error.
type fakeStore struct {
	*memstore.Store

	ttls      map[string]time.Duration
	getErr    error
	writeErr  error
	deleteErr error
}

// entry is a value written to the store before a test runs.
type entry struct {
	id   string
	data string
}

// newFakeStore returns a fakeStore holding only seeds, each stored for an hour.
func newFakeStore(t *testing.T, seeds ...entry) *fakeStore {
	t.Helper()

	store := &fakeStore{
		Store: memstore.New(),
		ttls:  make(map[string]time.Duration),
	}
	for _, e := range seeds {
		if _, err := store.Store.Create(t.Context(), e.id, []byte(e.data), time.Hour); err != nil {
			t.Fatalf("memstore.Store.Create: %v", err)
		}
	}

	return store
}

func (s *fakeStore) Get(ctx context.Context, id string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}

	return s.Store.Get(ctx, id)
}

func (s *fakeStore) Create(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if s.writeErr != nil {
		return false, s.writeErr
	}
	s.ttls[id] = ttl

	return s.Store.Create(ctx, id, data, ttl)
}

func (s *fakeStore) Update(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if s.writeErr != nil {
		return false, s.writeErr
	}
	s.ttls[id] = ttl

	return s.Store.Update(ctx, id, data, ttl)
}

func (s *fakeStore) Delete(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	return s.Store.Delete(ctx, id)
}

// statusRecordingWriter is an [httptest.ResponseRecorder] that records every
// status passed to WriteHeader, including those the recorder ignores because
// a status was already sent.
type statusRecordingWriter struct {
	*httptest.ResponseRecorder

	statuses []int
}

func (w *statusRecordingWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
	w.ResponseRecorder.WriteHeader(status)
}

// hijackableWriter is an [httptest.ResponseRecorder] whose connection can be
// hijacked. Hijack hands over conn and records the response headers as they
// were at that moment.
type hijackableWriter struct {
	*httptest.ResponseRecorder

	conn net.Conn

	hijacked       bool
	hijackedHeader http.Header
}

func (w *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	w.hijackedHeader = w.Header().Clone()

	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}
