package middleware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

const testCookieName = "tw_session"

func testOptions() SessionOptions {
	return SessionOptions{
		Name:             testCookieName,
		DefaultTTL:       time.Hour,
		AuthenticatedTTL: 30 * time.Minute,
	}
}

// fakeStore is a session.Store whose behavior is fully controlled by the test.
// It records the ID passed to Prepare and Drop, and the arguments of the most
// recent Commit call.
type fakeStore struct {
	prepareSnap *session.Snapshot
	prepareErr  error
	commitErr   error
	dropErr     error

	preparedID   string
	commitCalls  int
	committed    *session.Snapshot
	committedTTL time.Duration
	dropCalls    int
	droppedID    string
}

func (f *fakeStore) Prepare(_ context.Context, id string) (*session.Snapshot, error) {
	f.preparedID = id
	return f.prepareSnap, f.prepareErr
}

func (f *fakeStore) Commit(_ context.Context, snap *session.Snapshot, ttl time.Duration) error {
	f.commitCalls++
	f.committed = snap
	f.committedTTL = ttl
	return f.commitErr
}

func (f *fakeStore) Drop(_ context.Context, id string) error {
	f.dropCalls++
	f.droppedID = id
	return f.dropErr
}

func TestSession(t *testing.T) {
	t.Run("creates a new session and sets the cookie when no cookie is present", func(t *testing.T) {
		store := &fakeStore{}

		var gotSess *session.Session
		var gotOK bool
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotSess, gotOK = SessionFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if !gotOK {
			t.Fatal("want session in request context")
		}
		if gotSess.ID() == "" {
			t.Error("want non-empty session ID")
		}
		// No cookie on the request, so an empty ID is forwarded to the store.
		if store.preparedID != "" {
			t.Errorf("want prepared ID: %q, got: %q", "", store.preparedID)
		}

		// Session-scoped responses must not be cached.
		if want, got := "no-store", rec.Header().Get("Cache-Control"); want != got {
			t.Errorf("want Cache-Control: %q, got: %q", want, got)
		}

		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be set")
		}
		if cookie.Value != gotSess.ID() {
			t.Errorf("want cookie value: %q, got: %q", gotSess.ID(), cookie.Value)
		}
		if cookie.Path != "/" {
			t.Errorf("want cookie path: %q, got: %q", "/", cookie.Path)
		}
		if !cookie.HttpOnly {
			t.Error("want cookie to be HttpOnly")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("want cookie SameSite: %v, got: %v", http.SameSiteLaxMode, cookie.SameSite)
		}
		if cookie.Secure {
			t.Error("want cookie not to be Secure")
		}
		if cookie.MaxAge != int(time.Hour.Seconds()) {
			t.Errorf("want cookie Max-Age: %d, got: %d", int(time.Hour.Seconds()), cookie.MaxAge)
		}

		// The new session was committed under its own ID.
		if store.committed == nil {
			t.Fatal("want session to be committed")
		}
		if store.committed.ID != gotSess.ID() {
			t.Errorf("want committed ID: %q, got: %q", gotSess.ID(), store.committed.ID)
		}
	})

	t.Run("refreshes the cookie for an existing session", func(t *testing.T) {
		existing := session.New()
		store := &fakeStore{prepareSnap: existing.Snapshot()}

		var gotSess *session.Session
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotSess, _ = SessionFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: existing.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		// The cookie value is forwarded to the store and its session is loaded.
		if store.preparedID != existing.ID() {
			t.Errorf("want prepared ID: %q, got: %q", existing.ID(), store.preparedID)
		}
		if gotSess.ID() != existing.ID() {
			t.Errorf("want session ID: %q, got: %q", existing.ID(), gotSess.ID())
		}

		// The cookie is re-issued each request with the same value so its
		// Max-Age slides forward.
		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be refreshed")
		}
		if cookie.Value != existing.ID() {
			t.Errorf("want cookie value: %q, got: %q", existing.ID(), cookie.Value)
		}
		if cookie.MaxAge != int(time.Hour.Seconds()) {
			t.Errorf("want cookie Max-Age: %d, got: %d", int(time.Hour.Seconds()), cookie.MaxAge)
		}
	})

	t.Run("issues a new session when the cookie is unknown", func(t *testing.T) {
		// Prepare returns no snapshot for the unknown ID.
		store := &fakeStore{}

		var gotSess *session.Session
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotSess, _ = SessionFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: "does-not-exist"})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if store.preparedID != "does-not-exist" {
			t.Errorf("want prepared ID: %q, got: %q", "does-not-exist", store.preparedID)
		}
		if gotSess.ID() == "does-not-exist" {
			t.Error("want a new session ID, got the unknown cookie value")
		}

		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be set")
		}
		if cookie.Value != gotSess.ID() {
			t.Errorf("want cookie value: %q, got: %q", gotSess.ID(), cookie.Value)
		}
	})

	t.Run("marks the cookie Secure when configured", func(t *testing.T) {
		opts := testOptions()
		opts.Secure = true

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		Session(&fakeStore{}, opts)(okHandler()).ServeHTTP(rec, req)

		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be set")
		}
		if !cookie.Secure {
			t.Error("want cookie to be Secure")
		}
	})

	t.Run("honors the configured cookie name for reads and writes", func(t *testing.T) {
		const name = "sid"
		opts := testOptions()
		opts.Name = name

		// Unknown ID, so a new session is issued and a fresh cookie written.
		store := &fakeStore{}

		var gotSess *session.Session
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			gotSess, _ = SessionFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: name, Value: "old-value"})
		rec := httptest.NewRecorder()

		Session(store, opts)(next).ServeHTTP(rec, req)

		// Read: the value came from the "sid" cookie.
		if store.preparedID != "old-value" {
			t.Errorf("want prepared ID: %q, got: %q", "old-value", store.preparedID)
		}
		// Write: the refreshed cookie uses the configured name.
		cookie := findCookie(rec.Result().Cookies(), name)
		if cookie == nil {
			t.Fatalf("want cookie named %q to be set", name)
		}
		if cookie.Value != gotSess.ID() {
			t.Errorf("want cookie value: %q, got: %q", gotSess.ID(), cookie.Value)
		}
	})

	t.Run("commits with the default TTL for an unauthenticated session", func(t *testing.T) {
		store := &fakeStore{}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		Session(store, testOptions())(okHandler()).ServeHTTP(rec, req)

		if store.commitCalls != 1 {
			t.Errorf("want commit calls: %d, got: %d", 1, store.commitCalls)
		}
		if store.committedTTL != time.Hour {
			t.Errorf("want committed TTL: %v, got: %v", time.Hour, store.committedTTL)
		}
	})

	t.Run("commits with the authenticated TTL for an authenticated session", func(t *testing.T) {
		authed := session.New()
		authed.SetUser(&session.User{Email: "teacher@example.com"})
		store := &fakeStore{prepareSnap: authed.Snapshot()}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: authed.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(okHandler()).ServeHTTP(rec, req)

		if store.committedTTL != 30*time.Minute {
			t.Errorf("want committed TTL: %v, got: %v", 30*time.Minute, store.committedTTL)
		}
		if store.committed.User == nil {
			t.Error("want committed snapshot to have a user")
		}
	})

	t.Run("rotates the cookie when the handler authenticates the session", func(t *testing.T) {
		existing := session.New()
		store := &fakeStore{prepareSnap: existing.Snapshot()}

		// A login handler mutates the session and redirects, leaving the cookie
		// to the middleware.
		var gotSess *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotSess, _ = SessionFromContext(r.Context())
			gotSess.SetUser(&session.User{Email: "teacher@example.com"})
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: existing.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want status code: %d, got: %d", http.StatusSeeOther, rec.Code)
		}

		// SetUser rotated the ID, so the cookie written alongside the redirect
		// carries the post-handler ID and the authenticated Max-Age.
		if gotSess.ID() == existing.ID() {
			t.Fatal("want session ID to be rotated by SetUser")
		}
		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be set")
		}
		if cookie.Value != gotSess.ID() {
			t.Errorf("want cookie value: %q, got: %q", gotSess.ID(), cookie.Value)
		}
		if cookie.MaxAge != int((30 * time.Minute).Seconds()) {
			t.Errorf("want cookie Max-Age: %d, got: %d", int((30 * time.Minute).Seconds()), cookie.MaxAge)
		}

		// The store sees the rotated session and loses the entry it superseded.
		if store.committed == nil {
			t.Fatal("want session to be committed")
		}
		if store.committed.ID != gotSess.ID() {
			t.Errorf("want committed ID: %q, got: %q", gotSess.ID(), store.committed.ID)
		}
		if store.committed.User == nil {
			t.Error("want committed snapshot to have a user")
		}
		if store.committedTTL != 30*time.Minute {
			t.Errorf("want committed TTL: %v, got: %v", 30*time.Minute, store.committedTTL)
		}
		if store.dropCalls != 1 {
			t.Fatalf("want drop calls: %d, got: %d", 1, store.dropCalls)
		}
		if store.droppedID != existing.ID() {
			t.Errorf("want dropped ID: %q, got: %q", existing.ID(), store.droppedID)
		}
	})

	t.Run("leaves the old entry alone when the session is not rotated", func(t *testing.T) {
		existing := session.New()
		store := &fakeStore{prepareSnap: existing.Snapshot()}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: existing.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(okHandler()).ServeHTTP(rec, req)

		if store.dropCalls != 0 {
			t.Errorf("want drop calls: %d, got: %d", 0, store.dropCalls)
		}
	})

	t.Run("does not drop the old entry when the commit fails", func(t *testing.T) {
		existing := session.New()
		store := &fakeStore{
			prepareSnap: existing.Snapshot(),
			commitErr:   errors.New("commit failed"),
		}

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, _ := SessionFromContext(r.Context())
			sess.SetUser(&session.User{Email: "teacher@example.com"})
			w.WriteHeader(http.StatusNoContent)
		})

		var buf bytes.Buffer
		req := httptest.NewRequest(http.MethodPost, "/login", nil).WithContext(newCtxWithLogger(&buf))
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: existing.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		// Dropping the old entry after a failed commit would leave the client
		// holding a cookie for a session no store knows about.
		if store.dropCalls != 0 {
			t.Errorf("want drop calls: %d, got: %d", 0, store.dropCalls)
		}
	})

	t.Run("preserves the response and logs when Drop fails", func(t *testing.T) {
		existing := session.New()
		store := &fakeStore{
			prepareSnap: existing.Snapshot(),
			dropErr:     errors.New("drop failed"),
		}

		var gotSess *session.Session
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotSess, _ = SessionFromContext(r.Context())
			gotSess.SetUser(&session.User{Email: "teacher@example.com"})
			w.WriteHeader(http.StatusNoContent)
		})

		var buf bytes.Buffer
		req := httptest.NewRequest(http.MethodPost, "/login", nil).WithContext(newCtxWithLogger(&buf))
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: existing.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("want status code: %d, got: %d", http.StatusNoContent, rec.Code)
		}
		if !strings.Contains(buf.String(), "failed to drop rotated session") {
			t.Errorf("want drop failure to be logged, got: %q", buf.String())
		}

		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be set")
		}
		if cookie.Value != gotSess.ID() {
			t.Errorf("want cookie value: %q, got: %q", gotSess.ID(), cookie.Value)
		}
	})

	t.Run("saves once for a handler that writes more than once", func(t *testing.T) {
		store := &fakeStore{}

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("a"))
			_, _ = w.Write([]byte("b"))
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if store.commitCalls != 1 {
			t.Errorf("want commit calls: %d, got: %d", 1, store.commitCalls)
		}

		var cookies int
		for _, c := range rec.Result().Cookies() {
			if c.Name == testCookieName {
				cookies++
			}
		}
		if cookies != 1 {
			t.Errorf("want session cookies: %d, got: %d", 1, cookies)
		}
	})

	t.Run("defers the save when the handler writes an informational status", func(t *testing.T) {
		store := &fakeStore{}

		// A 1xx leaves the headers uncommitted, so the save has to wait for
		// the status that commits them. Sampling the store mid-handler is the
		// only way to see the deferral: by the time the request is over, an
		// eager save and a deferred one leave the same trace behind.
		var commitsAt1xx int
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusContinue)
			commitsAt1xx = store.commitCalls
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if commitsAt1xx != 0 {
			t.Errorf("want commit calls at the 1xx: %d, got: %d", 0, commitsAt1xx)
		}
		if store.commitCalls != 1 {
			t.Errorf("want commit calls: %d, got: %d", 1, store.commitCalls)
		}
	})

	t.Run("saves when the handler writes 101 Switching Protocols", func(t *testing.T) {
		store := &fakeStore{}

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusSwitchingProtocols)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if store.commitCalls != 1 {
			t.Errorf("want commit calls: %d, got: %d", 1, store.commitCalls)
		}
		if findCookie(rec.Result().Cookies(), testCookieName) == nil {
			t.Error("want session cookie to be set")
		}
	})

	t.Run("ignores session changes made after the response is written", func(t *testing.T) {
		existing := session.New()
		store := &fakeStore{prepareSnap: existing.Snapshot()}

		// The save runs on the first write, so this authentication lands too
		// late to reach the cookie or the store.
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))

			sess, _ := SessionFromContext(r.Context())
			sess.SetUser(&session.User{Email: "teacher@example.com"})
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: existing.ID()})
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		cookie := findCookie(rec.Result().Cookies(), testCookieName)
		if cookie == nil {
			t.Fatal("want session cookie to be set")
		}
		if cookie.Value != existing.ID() {
			t.Errorf("want cookie value: %q, got: %q", existing.ID(), cookie.Value)
		}
		if store.committed.User != nil {
			t.Error("want committed snapshot to have no user")
		}
		if store.committedTTL != time.Hour {
			t.Errorf("want committed TTL: %v, got: %v", time.Hour, store.committedTTL)
		}
		if store.dropCalls != 0 {
			t.Errorf("want drop calls: %d, got: %d", 0, store.dropCalls)
		}
	})

	t.Run("responds 500 and skips the handler when Prepare fails", func(t *testing.T) {
		store := &fakeStore{prepareErr: errors.New("prepare failed")}

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		var buf bytes.Buffer
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(newCtxWithLogger(&buf))
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("want status code: %d, got: %d", http.StatusInternalServerError, rec.Code)
		}
		if nextCalled {
			t.Error("want next handler not to be called")
		}
		if store.commitCalls != 0 {
			t.Errorf("want commit calls: %d, got: %d", 0, store.commitCalls)
		}
	})

	t.Run("preserves the response and logs when Commit fails", func(t *testing.T) {
		store := &fakeStore{commitErr: errors.New("commit failed")}

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("ok"))
		})

		var buf bytes.Buffer
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(newCtxWithLogger(&buf))
		rec := httptest.NewRecorder()

		Session(store, testOptions())(next).ServeHTTP(rec, req)

		// The handler's response is untouched by the failed commit.
		if rec.Code != http.StatusCreated {
			t.Errorf("want status code: %d, got: %d", http.StatusCreated, rec.Code)
		}
		if got := rec.Body.String(); got != "ok" {
			t.Errorf("want body: %q, got: %q", "ok", got)
		}
		if !strings.Contains(buf.String(), "failed to commit session") {
			t.Errorf("want commit failure to be logged, got: %q", buf.String())
		}
		if cookie := findCookie(rec.Result().Cookies(), testCookieName); cookie != nil {
			t.Errorf("want no session cookie, got: %q", cookie.Value)
		}
		// The header is set ahead of the commit, so it survives the failure.
		if want, got := "no-store", rec.Header().Get("Cache-Control"); want != got {
			t.Errorf("want Cache-Control: %q, got: %q", want, got)
		}
	})
}

// okHandler is a no-op next handler for tests that only exercise the middleware.
func okHandler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

// findCookie returns the cookie with the given name, or nil when none matches.
func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
