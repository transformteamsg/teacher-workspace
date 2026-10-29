package session

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/transformteamsg/teacher-workspace/server/internal/session/memstore"
)

func TestNew(t *testing.T) {
	t.Run("returns different IDs across calls", func(t *testing.T) {
		first := New()
		second := New()

		if want, got := first.ID(), second.ID(); want == got {
			t.Errorf("want second.ID(): != %q; got: %q", want, got)
		}
	})

	t.Run("returns an unauthenticated session", func(t *testing.T) {
		sess := New()

		if sess.IsAuthenticated() {
			t.Error("want sess.IsAuthenticated(): false; got: true")
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("returns a new session when the store has no entry under the ID", func(t *testing.T) {
		store := newFakeStore(t)

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

	t.Run("returns the error from store when store fails", func(t *testing.T) {
		store := newFakeStore(t)
		getErr := errors.New("store unavailable")
		store.getErr = getErr

		_, err := Load(t.Context(), store, "id-1")

		if !errors.Is(err, getErr) {
			t.Errorf("want err: %v; got: %v", getErr, err)
		}
	})

	t.Run("returns the session stored under the ID with a user", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		committed.SetUser(User{ID: "123456789012"})
		committed.Set("key", "value")
		token := committed.CSRFToken()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		loaded, err := Load(t.Context(), store, committed.ID())

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := committed.ID(), loaded.ID(); want != got {
			t.Errorf("want loaded.ID(): %q; got: %q", want, got)
		}
		if !loaded.VerifyCSRFToken(token) {
			t.Error("want loaded.VerifyCSRFToken(token): true; got: false")
		}
		user, ok := loaded.User()
		if !ok {
			t.Fatal("want loaded.User() ok: true; got: false")
		}
		if want := (User{ID: "123456789012"}); !reflect.DeepEqual(want, user) {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
		val, ok := loaded.Get[string]("key")
		if !ok {
			t.Fatal("want loaded.Get[string](\"key\") ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("returns the session stored under the ID without a user", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		loaded, err := Load(t.Context(), store, committed.ID())

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		// A fresh session from New would also be unauthenticated.
		if want, got := committed.ID(), loaded.ID(); want != got {
			t.Fatalf("want loaded.ID(): %q; got: %q", want, got)
		}
		if loaded.IsAuthenticated() {
			t.Error("want loaded.IsAuthenticated(): false; got: true")
		}
	})

	t.Run("returns an error wrapping ErrUndecodable", func(t *testing.T) {
		cases := []struct {
			name  string
			entry string
		}{
			{
				name:  "for an entry that is not JSON",
				entry: `{`,
			},
			{
				name:  "for an entry holding another ID",
				entry: `{"id":"id-2","csrf_token":"csrf-1"}`,
			},
			{
				name:  "for an entry without a CSRF token",
				entry: `{"id":"id-1"}`,
			},
		}

		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				store := newFakeStore(t, seed{
					id:   "id-1",
					data: test.entry,
				})

				_, err := Load(t.Context(), store, "id-1")

				if !errors.Is(err, ErrUndecodable) {
					t.Errorf("want err: %v; got: %v", ErrUndecodable, err)
				}
			})
		}
	})
}

func TestCommit(t *testing.T) {
	t.Run("writes a new session under its ID", func(t *testing.T) {
		store := newFakeStore(t)
		sess := New()
		sess.SetUser(User{
			ID:         "123456789012",
			Name:       "John Smith",
			Email:      "a@example.com",
			Role:       "ROLE_TEACHER",
			Attributes: []string{"ATTR_CCE"},
		})
		sess.Set("key", "value")
		token := sess.CSRFToken()

		err := Commit(t.Context(), store, sess, 2*time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		loaded, err := Load(t.Context(), store, sess.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if want, got := sess.ID(), loaded.ID(); want != got {
			t.Fatalf("want loaded.ID(): %q; got: %q", want, got)
		}
		if !loaded.VerifyCSRFToken(token) {
			t.Error("want loaded.VerifyCSRFToken(token): true; got: false")
		}
		user, ok := loaded.User()
		if !ok {
			t.Fatal("want loaded.User() ok: true; got: false")
		}
		if want := (User{
			ID:         "123456789012",
			Name:       "John Smith",
			Email:      "a@example.com",
			Role:       "ROLE_TEACHER",
			Attributes: []string{"ATTR_CCE"},
		}); !reflect.DeepEqual(want, user) {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
		val, ok := loaded.Get[string]("key")
		if !ok {
			t.Fatal("want loaded.Get[string](\"key\") ok: true; got: false")
		}
		if want := "value"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
		ttl, ok := store.ttls[sess.ID()]
		if !ok {
			t.Fatal("want store.ttls[sess.ID()] ok: true; got: false")
		}
		if want := 2 * time.Hour; want != ttl {
			t.Errorf("want ttl: %v; got: %v", want, ttl)
		}
	})

	t.Run("rejects a session whose ID is taken by another session", func(t *testing.T) {
		sess := New()
		store := newFakeStore(t, seed{
			id:   sess.ID(),
			data: "other",
		})

		err := Commit(t.Context(), store, sess, time.Hour)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if errors.Is(err, ErrNotFound) {
			t.Errorf("want err: not %v; got: %v", ErrNotFound, err)
		}
		if errors.Is(err, ErrSupersededNotDeleted) {
			t.Errorf("want err: not %v; got: %v", ErrSupersededNotDeleted, err)
		}
		entry, err := store.Get(t.Context(), sess.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "other", string(entry); want != got {
			t.Errorf("want entry: %q; got: %q", want, got)
		}
	})

	t.Run("rejects a session with a value that cannot be encoded as JSON", func(t *testing.T) {
		store := newFakeStore(t)
		sess := New()
		sess.Set("key", make(chan int))

		err := Commit(t.Context(), store, sess, time.Hour)

		if err == nil {
			t.Error("want err: non-nil; got: nil")
		}
		entry, err := store.Get(t.Context(), sess.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry != nil {
			t.Error("want entry: nil; got: non-nil")
		}
	})

	t.Run("returns the error from store when it cannot write a new session", func(t *testing.T) {
		store := newFakeStore(t)
		createErr := errors.New("store unavailable")
		store.createErr = createErr
		sess := New()

		err := Commit(t.Context(), store, sess, time.Hour)

		if !errors.Is(err, createErr) {
			t.Errorf("want err: %v; got: %v", createErr, err)
		}
	})

	t.Run("writes the session under its new ID after the session's ID changes", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.SetUser(User{Email: "a@example.com"})
		if loaded.ID() == committed.ID() {
			t.Fatal("loaded.SetUser: ID unchanged")
		}

		err = Commit(t.Context(), store, loaded, 2*time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		entry, err := store.Get(t.Context(), loaded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry == nil {
			t.Error("want entry: non-nil; got: nil")
		}
		ttl, ok := store.ttls[loaded.ID()]
		if !ok {
			t.Fatal("want store.ttls[loaded.ID()] ok: true; got: false")
		}
		if want := 2 * time.Hour; want != ttl {
			t.Errorf("want ttl: %v; got: %v", want, ttl)
		}
	})

	t.Run("removes the superseded entry after the session's ID changes", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.SetUser(User{Email: "a@example.com"})

		err = Commit(t.Context(), store, loaded, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		entry, err := store.Get(t.Context(), committed.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry != nil {
			t.Error("want entry: nil; got: non-nil")
		}
	})

	t.Run("returns ErrSupersededNotDeleted wrapped with the store's error when the superseded entry cannot be removed", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.SetUser(User{Email: "a@example.com"})
		deleteErr := errors.New("store unavailable")
		store.deleteErr = deleteErr

		err = Commit(t.Context(), store, loaded, time.Hour)

		if !errors.Is(err, ErrSupersededNotDeleted) {
			t.Errorf("want err: %v; got: %v", ErrSupersededNotDeleted, err)
		}
		if !errors.Is(err, deleteErr) {
			t.Errorf("want err: %v; got: %v", deleteErr, err)
		}
	})

	t.Run("writes the session under its new ID when the superseded entry cannot be removed", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.SetUser(User{Email: "a@example.com"})
		if loaded.ID() == committed.ID() {
			t.Fatal("loaded.SetUser: ID unchanged")
		}
		store.deleteErr = errors.New("store unavailable")

		_ = Commit(t.Context(), store, loaded, time.Hour)

		entry, err := store.Get(t.Context(), loaded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry == nil {
			t.Error("want entry: non-nil; got: nil")
		}
	})

	t.Run("writes the session under its new ID even if the superseded entry is no longer in store", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if err := store.Delete(t.Context(), committed.ID()); err != nil {
			t.Fatalf("store.Delete: %v", err)
		}
		superseded, err := store.Get(t.Context(), committed.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if superseded != nil {
			t.Fatal("store.Delete: entry still in store")
		}
		loaded.SetUser(User{Email: "a@example.com"})

		err = Commit(t.Context(), store, loaded, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		entry, err := store.Get(t.Context(), loaded.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry == nil {
			t.Error("want entry: non-nil; got: nil")
		}
	})

	t.Run("writes the session under its ID when its ID is unchanged since it was loaded", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.Set("key", "new")

		err = Commit(t.Context(), store, loaded, 2*time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		reloaded, err := Load(t.Context(), store, loaded.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		val, ok := reloaded.Get[string]("key")
		if !ok {
			t.Fatal("want reloaded.Get[string](\"key\") ok: true; got: false")
		}
		if want := "new"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
		ttl, ok := store.ttls[loaded.ID()]
		if !ok {
			t.Fatal("want store.ttls[loaded.ID()] ok: true; got: false")
		}
		if want := 2 * time.Hour; want != ttl {
			t.Errorf("want ttl: %v; got: %v", want, ttl)
		}
	})

	t.Run("writes the session under its ID when its ID is unchanged since it was last committed", func(t *testing.T) {
		store := newFakeStore(t)
		sess := New()
		if err := Commit(t.Context(), store, sess, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		sess.Set("key", "new")

		err := Commit(t.Context(), store, sess, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		reloaded, err := Load(t.Context(), store, sess.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		val, ok := reloaded.Get[string]("key")
		if !ok {
			t.Fatal("want reloaded.Get[string](\"key\") ok: true; got: false")
		}
		if want := "new"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("writes the session under its ID when its ID is unchanged since a commit that could not remove the superseded entry", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.SetUser(User{Email: "a@example.com"})
		store.deleteErr = errors.New("store unavailable")
		if err := Commit(t.Context(), store, loaded, time.Hour); !errors.Is(err, ErrSupersededNotDeleted) {
			t.Fatalf("Commit: err not wrapping ErrSupersededNotDeleted: %v", err)
		}
		loaded.Set("key", "new")

		err = Commit(t.Context(), store, loaded, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		reloaded, err := Load(t.Context(), store, loaded.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		val, ok := reloaded.Get[string]("key")
		if !ok {
			t.Fatal("want reloaded.Get[string](\"key\") ok: true; got: false")
		}
		if want := "new"; want != val {
			t.Errorf("want val: %q; got: %q", want, val)
		}
	})

	t.Run("rejects a session whose ID is unchanged but whose entry is no longer in store", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if err := store.Delete(t.Context(), committed.ID()); err != nil {
			t.Fatalf("store.Delete: %v", err)
		}

		err = Commit(t.Context(), store, loaded, time.Hour)

		if !errors.Is(err, ErrNotFound) {
			t.Errorf("want err: %v; got: %v", ErrNotFound, err)
		}
		entry, err := store.Get(t.Context(), committed.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry != nil {
			t.Error("want entry: nil; got: non-nil")
		}
	})

	t.Run("returns the error from store when it cannot write a session whose ID is unchanged", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		updateErr := errors.New("store unavailable")
		store.updateErr = updateErr

		err = Commit(t.Context(), store, loaded, time.Hour)

		if !errors.Is(err, updateErr) {
			t.Errorf("want err: %v; got: %v", updateErr, err)
		}
	})

	t.Run("removes a destroyed session", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.SetUser(User{Email: "a@example.com"})
		if loaded.ID() == committed.ID() {
			t.Fatal("loaded.SetUser: ID unchanged")
		}
		loaded.Destroy()

		err = Commit(t.Context(), store, loaded, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		entry, err := store.Get(t.Context(), committed.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry != nil {
			t.Error("want entry: nil; got: non-nil")
		}
	})

	t.Run("does not write a destroyed session", func(t *testing.T) {
		store := newFakeStore(t)
		sess := New()
		sess.Destroy()

		err := Commit(t.Context(), store, sess, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		entry, err := store.Get(t.Context(), sess.ID())
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if entry != nil {
			t.Error("want entry: nil; got: non-nil")
		}
	})

	t.Run("returns the error from store when it cannot remove a destroyed session", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		loaded.Destroy()
		deleteErr := errors.New("store unavailable")
		store.deleteErr = deleteErr

		err = Commit(t.Context(), store, loaded, time.Hour)

		if !errors.Is(err, deleteErr) {
			t.Errorf("want err: %v; got: %v", deleteErr, err)
		}
	})
}

func TestSession_CSRFToken(t *testing.T) {
	t.Run("returns a different token on each call", func(t *testing.T) {
		sess := New()

		first := sess.CSRFToken()
		second := sess.CSRFToken()

		if first == second {
			t.Errorf("want second: != %q; got: %q", first, second)
		}
	})
}

func TestSession_VerifyCSRFToken(t *testing.T) {
	t.Run("accepts the latest token", func(t *testing.T) {
		sess := New()
		token := sess.CSRFToken()

		accepted := sess.VerifyCSRFToken(token)

		if !accepted {
			t.Error("want accepted: true; got: false")
		}
	})

	t.Run("accepts an older token", func(t *testing.T) {
		sess := New()
		token := sess.CSRFToken()
		sess.CSRFToken()

		accepted := sess.VerifyCSRFToken(token)

		if !accepted {
			t.Error("want accepted: true; got: false")
		}
	})

	t.Run("accepts a token issued after SetUser", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})
		token := sess.CSRFToken()

		accepted := sess.VerifyCSRFToken(token)

		if !accepted {
			t.Error("want accepted: true; got: false")
		}
	})

	t.Run("rejects a token from another session", func(t *testing.T) {
		sess := New()
		token := New().CSRFToken()

		accepted := sess.VerifyCSRFToken(token)

		if accepted {
			t.Error("want accepted: false; got: true")
		}
	})

	t.Run("rejects an empty token", func(t *testing.T) {
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
		if want := (User{Email: "a@example.com"}); !reflect.DeepEqual(want, user) {
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

	t.Run("returns a copy whose attributes are not shared with the session", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Attributes: []string{"ATTR_A"}})

		user, ok := sess.User()

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		user.Attributes[0] = "ATTR_B"

		user, ok = sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want, got := []string{"ATTR_A"}, user.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want user.Attributes: %#v; got: %#v", want, got)
		}
	})

	t.Run("returns the attributes the user was attached with", func(t *testing.T) {
		tests := []struct {
			name           string
			attributes     []string
			wantAttributes []string
		}{
			{
				name:           "when nil",
				attributes:     nil,
				wantAttributes: nil,
			},
			{
				name:           "when empty",
				attributes:     []string{},
				wantAttributes: []string{},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				sess := New()
				sess.SetUser(User{Attributes: test.attributes})

				user, ok := sess.User()

				if !ok {
					t.Fatal("want ok: true; got: false")
				}
				if want, got := test.wantAttributes, user.Attributes; !reflect.DeepEqual(want, got) {
					t.Errorf("want user.Attributes: %#v; got: %#v", want, got)
				}
			})
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
	t.Run("returns the value stored under the key", func(t *testing.T) {
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

	t.Run("reports false for a missing key", func(t *testing.T) {
		sess := New()
		sess.Set("other", "value")

		_, ok := sess.Get[string]("key")

		if ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("reports false for a value of another type", func(t *testing.T) {
		sess := New()
		sess.Set("key", 1)

		_, ok := sess.Get[string]("key")

		if ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("returns numbers as float64 on a loaded session", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		committed.Set("key", 1)
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		val, ok := loaded.Get[float64]("key")

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := 1.0; want != val {
			t.Errorf("want val: %v; got: %v", want, val)
		}
	})

	t.Run("returns structs as map[string]any on a loaded session", func(t *testing.T) {
		store := newFakeStore(t)
		committed := New()
		committed.Set("key", struct{ Name string }{Name: "a"})
		if err := Commit(t.Context(), store, committed, time.Hour); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		loaded, err := Load(t.Context(), store, committed.ID())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		val, ok := loaded.Get[map[string]any]("key")

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := (map[string]any{"Name": "a"}); !reflect.DeepEqual(want, val) {
			t.Errorf("want val: %v; got: %v", want, val)
		}
	})
}

func TestSession_GetAndDelete(t *testing.T) {
	t.Run("returns the value stored under the key", func(t *testing.T) {
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

	t.Run("removes the value stored under the key", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.GetAndDelete[string]("key")

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})

	t.Run("reports false for a missing key", func(t *testing.T) {
		sess := New()
		sess.Set("other", "value")

		_, ok := sess.GetAndDelete[string]("key")

		if ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("removes a value of another type", func(t *testing.T) {
		sess := New()
		sess.Set("key", 1)

		sess.GetAndDelete[string]("key")

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

func TestSession_Set(t *testing.T) {
	t.Run("stores the value under the key", func(t *testing.T) {
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
	t.Run("removes the value stored under the key", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.Delete("key")

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

func TestSession_SetUser(t *testing.T) {
	t.Run("attaches the user", func(t *testing.T) {
		sess := New()

		sess.SetUser(User{Email: "a@example.com"})

		user, ok := sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want := (User{Email: "a@example.com"}); !reflect.DeepEqual(want, user) {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
	})

	t.Run("replaces the user already attached", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})

		sess.SetUser(User{Email: "b@example.com"})

		user, ok := sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want := (User{Email: "b@example.com"}); !reflect.DeepEqual(want, user) {
			t.Errorf("want user: %+v; got: %+v", want, user)
		}
	})

	t.Run("attaches a copy whose attributes are not shared with the caller", func(t *testing.T) {
		sess := New()
		attached := User{Attributes: []string{"ATTR_A"}}

		sess.SetUser(attached)
		attached.Attributes[0] = "ATTR_B"

		user, ok := sess.User()
		if !ok {
			t.Fatal("want sess.User() ok: true; got: false")
		}
		if want, got := []string{"ATTR_A"}, user.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want user.Attributes: %#v; got: %#v", want, got)
		}
	})

	t.Run("gives the session a new ID", func(t *testing.T) {
		sess := New()
		oldID := sess.ID()

		sess.SetUser(User{Email: "a@example.com"})

		if got := sess.ID(); oldID == got {
			t.Errorf("want sess.ID(): != %q; got: %q", oldID, got)
		}
	})

	t.Run("invalidates the CSRF tokens returned so far", func(t *testing.T) {
		sess := New()
		token := sess.CSRFToken()

		sess.SetUser(User{Email: "a@example.com"})

		if sess.VerifyCSRFToken(token) {
			t.Error("want sess.VerifyCSRFToken(token): false; got: true")
		}
	})

	t.Run("discards the session's data", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.SetUser(User{Email: "a@example.com"})

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

func TestSession_Destroy(t *testing.T) {
	t.Run("detaches the session's user", func(t *testing.T) {
		sess := New()
		sess.SetUser(User{Email: "a@example.com"})

		sess.Destroy()

		if sess.IsAuthenticated() {
			t.Error("want sess.IsAuthenticated(): false; got: true")
		}
	})

	t.Run("discards the session's data", func(t *testing.T) {
		sess := New()
		sess.Set("key", "value")

		sess.Destroy()

		if _, ok := sess.Get[any]("key"); ok {
			t.Error("want sess.Get[any](\"key\") ok: false; got: true")
		}
	})
}

func TestSession_IsDestroyed(t *testing.T) {
	t.Run("reports false before Destroy", func(t *testing.T) {
		sess := New()

		destroyed := sess.IsDestroyed()

		if destroyed {
			t.Error("want destroyed: false; got: true")
		}
	})

	t.Run("reports true after Destroy", func(t *testing.T) {
		sess := New()
		sess.Destroy()

		destroyed := sess.IsDestroyed()

		if !destroyed {
			t.Error("want destroyed: true; got: false")
		}
	})
}

// Asserts at compile time that *fakeStore satisfies Store.
var _ Store = (*fakeStore)(nil)

// fakeStore is a Store backed by a memstore.Store. It records the TTL of each
// Create and Update call. When getErr, createErr, updateErr or deleteErr is
// set, Get, Create, Update or Delete returns it without touching the entries.
type fakeStore struct {
	*memstore.Store

	ttls      map[string]time.Duration
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

// seed is an entry written to a fakeStore before a test runs.
type seed struct {
	id   string
	data string
}

// newFakeStore returns a fakeStore holding only seeds, each stored for an hour.
func newFakeStore(t *testing.T, seeds ...seed) *fakeStore {
	t.Helper()

	store := &fakeStore{
		Store: memstore.New(),
		ttls:  make(map[string]time.Duration),
	}
	for _, s := range seeds {
		if _, err := store.Store.Create(t.Context(), s.id, []byte(s.data), time.Hour); err != nil {
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
	if s.createErr != nil {
		return false, s.createErr
	}
	s.ttls[id] = ttl

	return s.Store.Create(ctx, id, data, ttl)
}

func (s *fakeStore) Update(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if s.updateErr != nil {
		return false, s.updateErr
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
