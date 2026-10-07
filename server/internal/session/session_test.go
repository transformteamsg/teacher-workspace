package session

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("returns different IDs across calls", func(t *testing.T) {
		first := New()
		second := New()

		if want, got := first.ID(), second.ID(); want == got {
			t.Errorf("want second.ID(): != %q; got: %q", want, got)
		}
	})

	t.Run("returns unauthenticated session", func(t *testing.T) {
		sess := New()

		if sess.IsAuthenticated() {
			t.Error("want sess.IsAuthenticated(): false; got: true")
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("returns new session when store has no entry", func(t *testing.T) {
		store := &fakeStore{}

		sess, err := Load(t.Context(), store, "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "id-1", sess.ID(); want == got {
			t.Errorf("want sess.ID(): != %q; got: %q", want, got)
		}
		if sess.IsAuthenticated() {
			t.Error("want sess.IsAuthenticated(): false; got: true")
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		storeErr := errors.New("store unavailable")
		store := &fakeStore{prepareErr: storeErr}

		_, err := Load(t.Context(), store, "id-1")

		if !errors.Is(err, storeErr) {
			t.Errorf("want err: %v; got: %v", storeErr, err)
		}
	})

	t.Run("returns session stored under ID with a user", func(t *testing.T) {
		store := &fakeStore{}
		saved := New()
		saved.SetUser(User{Email: "a@example.com"})
		saved.Set("key", "value")
		token := saved.CSRFToken()
		if err := Save(t.Context(), store, saved, time.Hour); err != nil {
			t.Fatalf("Save: %v", err)
		}

		sess, err := Load(t.Context(), store, saved.ID())

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := saved.ID(), sess.ID(); want != got {
			t.Errorf("want sess.ID(): %q; got: %q", want, got)
		}
		if !sess.VerifyCSRFToken(token) {
			t.Error("want sess.VerifyCSRFToken(token): true; got: false")
		}
		user, ok := sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want := (User{Email: "a@example.com"}); want != user {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
		val, ok := sess.Get[string]("key")
		if !ok {
			t.Fatal("want sess.Get[string](\"key\") ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("returns session stored under ID without a user", func(t *testing.T) {
		store := &fakeStore{}
		saved := New()
		if err := Save(t.Context(), store, saved, time.Hour); err != nil {
			t.Fatalf("Save: %v", err)
		}

		sess, err := Load(t.Context(), store, saved.ID())

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		// A fresh session from New would also be unauthenticated.
		if want, got := saved.ID(), sess.ID(); want != got {
			t.Fatalf("want sess.ID(): %q; got: %q", want, got)
		}
		if sess.IsAuthenticated() {
			t.Error("want sess.IsAuthenticated(): false; got: true")
		}
	})

	t.Run("returns numbers as float64", func(t *testing.T) {
		store := &fakeStore{}
		saved := New()
		saved.Set("key", 1)
		if err := Save(t.Context(), store, saved, time.Hour); err != nil {
			t.Fatalf("Save: %v", err)
		}

		sess, err := Load(t.Context(), store, saved.ID())

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := sess.Get[float64]("key"); !ok {
			t.Error("want sess.Get[float64](\"key\") ok: true; got: false")
		}
	})

	t.Run("returns structs as map[string]any", func(t *testing.T) {
		store := &fakeStore{}
		saved := New()
		saved.Set("key", struct{ Name string }{Name: "a"})
		if err := Save(t.Context(), store, saved, time.Hour); err != nil {
			t.Fatalf("Save: %v", err)
		}

		sess, err := Load(t.Context(), store, saved.ID())

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := sess.Get[map[string]any]("key"); !ok {
			t.Error("want sess.Get[map[string]any](\"key\") ok: true; got: false")
		}
	})

	t.Run("returns error wrapping ErrUndecodable", func(t *testing.T) {
		cases := []struct {
			name  string
			entry []byte
		}{
			{
				name:  "for invalid JSON",
				entry: []byte(`{`),
			},
			{
				name:  "for entry stored under another ID",
				entry: []byte(`{"id":"id-2","csrf_token":"csrf-1"}`),
			},
			{
				name:  "for entry without CSRF token",
				entry: []byte(`{"id":"id-1"}`),
			},
		}

		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				store := &fakeStore{entries: map[string][]byte{"id-1": test.entry}}

				_, err := Load(t.Context(), store, "id-1")

				if !errors.Is(err, ErrUndecodable) {
					t.Errorf("want err: %v; got: %v", ErrUndecodable, err)
				}
			})
		}
	})
}

func TestSave(t *testing.T) {
	t.Run("writes session under its ID with TTL", func(t *testing.T) {
		store := &fakeStore{}
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})
		sess.Set("key", "value")

		err := Save(t.Context(), store, sess, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		want := map[string]any{
			"id":         sess.ID(),
			"csrf_token": sess.csrfToken,
			"user":       map[string]any{"email": "a@example.com"},
			"data":       map[string]any{"key": "value"},
		}
		entry, ok := store.entries[sess.ID()]
		if !ok {
			t.Fatal("want store.entries[sess.ID()] ok: true; got: false")
		}
		var stored map[string]any
		if err := json.Unmarshal(entry, &stored); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if !reflect.DeepEqual(want, stored) {
			t.Errorf("want store entry: %v; got: %v", want, stored)
		}
		ttl, ok := store.ttls[sess.ID()]
		if !ok {
			t.Fatal("want store.ttls[sess.ID()] ok: true; got: false")
		}
		if want := time.Hour; want != ttl {
			t.Errorf("want ttl: %v; got: %v", want, ttl)
		}
	})

	t.Run("returns error when a value cannot be encoded as JSON", func(t *testing.T) {
		store := &fakeStore{}
		sess := New()
		sess.Set("key", make(chan int))

		err := Save(t.Context(), store, sess, time.Hour)

		if err == nil {
			t.Error("want err: non-nil; got: nil")
		}
	})

	t.Run("returns store error", func(t *testing.T) {
		commitErr := errors.New("store unavailable")
		store := &fakeStore{commitErr: commitErr}
		sess := New()

		err := Save(t.Context(), store, sess, time.Hour)

		if !errors.Is(err, commitErr) {
			t.Errorf("want err: %v; got: %v", commitErr, err)
		}
	})
}

func TestSession_CSRFToken(t *testing.T) {
	t.Run("returns different token on each call", func(t *testing.T) {
		sess := New()

		first := sess.CSRFToken()
		second := sess.CSRFToken()

		if first == second {
			t.Errorf("want second: != %q; got: %q", first, second)
		}
	})

	t.Run("returns token that VerifyCSRFToken accepts", func(t *testing.T) {
		sess := New()

		token := sess.CSRFToken()

		if !sess.VerifyCSRFToken(token) {
			t.Error("want sess.VerifyCSRFToken(token): true; got: false")
		}
	})

	t.Run("keeps earlier tokens valid", func(t *testing.T) {
		sess := New()
		token := sess.CSRFToken()

		sess.CSRFToken()

		if !sess.VerifyCSRFToken(token) {
			t.Error("want sess.VerifyCSRFToken(token): true; got: false")
		}
	})
}

func TestSession_VerifyCSRFToken(t *testing.T) {
	t.Run("accepts token from this session", func(t *testing.T) {
		sess := New()
		token := sess.CSRFToken()

		accepted := sess.VerifyCSRFToken(token)

		if !accepted {
			t.Error("want accepted: true; got: false")
		}
	})

	t.Run("rejects token from another session", func(t *testing.T) {
		sess := New()
		token := New().CSRFToken()

		accepted := sess.VerifyCSRFToken(token)

		if accepted {
			t.Error("want accepted: false; got: true")
		}
	})

	t.Run("rejects empty token", func(t *testing.T) {
		sess := New()

		accepted := sess.VerifyCSRFToken("")

		if accepted {
			t.Error("want accepted: false; got: true")
		}
	})
}

func TestSession_User(t *testing.T) {
	t.Run("returns the user when a user is attached", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})

		user, ok := sess.User()

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := (User{Email: "a@example.com"}); want != user {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
	})

	t.Run("reports false when no user is attached", func(t *testing.T) {
		sess := New()

		_, ok := sess.User()

		if ok {
			t.Error("want ok: false; got: true")
		}
	})
}

func TestSession_IsAuthenticated(t *testing.T) {
	t.Run("reports true when a user is attached", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})

		authenticated := sess.IsAuthenticated()

		if !authenticated {
			t.Error("want authenticated: true; got: false")
		}
	})

	t.Run("reports false when no user is attached", func(t *testing.T) {
		sess := New()

		authenticated := sess.IsAuthenticated()

		if authenticated {
			t.Error("want authenticated: false; got: true")
		}
	})
}

func TestSession_Get(t *testing.T) {
	t.Run("returns value stored under key", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		val, ok := sess.Get[string]("key")

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("reports false for missing key", func(t *testing.T) {
		sess := New()
		sess.Set("other", "value")

		_, ok := sess.Get[string]("key")

		if ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("reports false for value of another type", func(t *testing.T) {
		sess := New()
		sess.Set("key", 1)

		_, ok := sess.Get[string]("key")

		if ok {
			t.Error("want ok: false; got: true")
		}
	})
}

func TestSession_GetAndDelete(t *testing.T) {
	t.Run("returns value stored under key", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		val, ok := sess.GetAndDelete[string]("key")

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("removes value stored under key", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.GetAndDelete[string]("key")

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})

	t.Run("reports false for missing key", func(t *testing.T) {
		sess := New()
		sess.Set("other", "value")

		_, ok := sess.GetAndDelete[string]("key")

		if ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("removes value of another type", func(t *testing.T) {
		sess := New()
		sess.Set("key", 1)

		sess.GetAndDelete[string]("key")

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

func TestSession_Set(t *testing.T) {
	t.Run("stores value under key", func(t *testing.T) {
		sess := New()

		sess.Set("key", "value")

		val, ok := sess.Get[string]("key")
		if !ok {
			t.Fatal("want sess.Get[string](\"key\") ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("keeps values under other keys", func(t *testing.T) {
		sess := New()
		sess.Set("other", "value")

		sess.Set("key", "new")

		val, ok := sess.Get[string]("other")
		if !ok {
			t.Fatal("want sess.Get[string](\"other\") ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})
}

func TestSession_Delete(t *testing.T) {
	t.Run("removes value stored under key", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.Delete("key")

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

func TestSession_SetUser(t *testing.T) {
	t.Run("attaches user", func(t *testing.T) {
		sess := New()

		sess.SetUser(User{Email: "a@example.com"})

		user, ok := sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want := (User{Email: "a@example.com"}); want != user {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
	})

	t.Run("replaces existing user", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})

		sess.SetUser(User{Email: "b@example.com"})

		user, ok := sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want := (User{Email: "b@example.com"}); want != user {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
	})

	t.Run("gives session a new ID", func(t *testing.T) {
		sess := New()
		oldID := sess.ID()

		sess.SetUser(User{Email: "a@example.com"})

		if got := sess.ID(); oldID == got {
			t.Errorf("want sess.ID(): != %q; got: %q", oldID, got)
		}
	})

	t.Run("invalidates earlier CSRF tokens", func(t *testing.T) {
		sess := New()
		token := sess.CSRFToken()

		sess.SetUser(User{Email: "a@example.com"})

		if sess.VerifyCSRFToken(token) {
			t.Error("want sess.VerifyCSRFToken(token): false; got: true")
		}
	})

	t.Run("issues new CSRF tokens that VerifyCSRFToken accepts", func(t *testing.T) {
		sess := New()

		sess.SetUser(User{Email: "a@example.com"})

		if !sess.VerifyCSRFToken(sess.CSRFToken()) {
			t.Error("want sess.VerifyCSRFToken(sess.CSRFToken()): true; got: false")
		}
	})

	t.Run("discards data", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.SetUser(User{Email: "a@example.com"})

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

// Asserts at compile time that *fakeStore satisfies Store.
var _ Store = (*fakeStore)(nil)

// fakeStore is a Store that holds entries in a map and never expires them. It
// records the TTL of each committed entry. When prepareErr or commitErr is set,
// Prepare or Commit returns it without touching the entries.
type fakeStore struct {
	entries    map[string][]byte
	ttls       map[string]time.Duration
	prepareErr error
	commitErr  error
}

func (s *fakeStore) Prepare(_ context.Context, id string) ([]byte, error) {
	if s.prepareErr != nil {
		return nil, s.prepareErr
	}

	return s.entries[id], nil
}

func (s *fakeStore) Commit(_ context.Context, id string, data []byte, ttl time.Duration) error {
	if s.commitErr != nil {
		return s.commitErr
	}

	if s.entries == nil {
		s.entries = make(map[string][]byte)
	}
	if s.ttls == nil {
		s.ttls = make(map[string]time.Duration)
	}
	s.entries[id] = data
	s.ttls[id] = ttl

	return nil
}

func (s *fakeStore) Drop(_ context.Context, id string) error {
	delete(s.entries, id)
	delete(s.ttls, id)

	return nil
}
