package session

import "testing"

func TestNew(t *testing.T) {
	t.Run("returns an unauthenticated session with non-empty ID and CSRF token", func(t *testing.T) {
		sess := New()

		if got := sess.id; got == "" {
			t.Errorf("want: non-empty; got: %q", got)
		}
		if got := sess.csrfToken; got == "" {
			t.Errorf("want: non-empty; got: %q", got)
		}
		if got := sess.user; got != nil {
			t.Errorf("want: nil; got: %+v", got)
		}
	})
}

func TestFromSnapshot(t *testing.T) {
	t.Run("preserves all fields", func(t *testing.T) {
		user := &User{Email: "alice@example.com"}
		snapshot := &Snapshot{
			ID:        "id-123",
			CSRFToken: "csrf-456",
			User:      user,
			Data:      map[string]any{"k": "v"},
		}

		sess := FromSnapshot(snapshot)

		if want, got := "id-123", sess.id; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "csrf-456", sess.csrfToken; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if got := sess.user; user != got {
			t.Errorf("want: %+v; got: %+v", user, got)
		}

		value, ok := sess.data["k"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})

	t.Run("panics on nil snapshot", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("want: panic; got: nil")
			}
		}()

		FromSnapshot(nil)
	})
}

func TestSession_Snapshot(t *testing.T) {
	t.Run("reflects ID, CSRF token, user, and data", func(t *testing.T) {
		user := &User{Email: "alice@example.com"}
		sess := &Session{
			id:        "id-1",
			csrfToken: "csrf-1",
			user:      user,
			data:      map[string]any{"k": "v"},
		}

		snapshot := sess.Snapshot()

		if want, got := "id-1", snapshot.ID; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "csrf-1", snapshot.CSRFToken; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if got := snapshot.User; user != got {
			t.Errorf("want: %+v; got: %+v", user, got)
		}

		value, ok := snapshot.Data["k"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})
}

func TestSession_User(t *testing.T) {
	t.Run("returns the stored user", func(t *testing.T) {
		user := &User{Email: "alice@example.com"}
		sess := &Session{user: user}

		if got := sess.User(); user != got {
			t.Errorf("want: %+v; got: %+v", user, got)
		}
	})
}

func TestSession_IsAuthenticated(t *testing.T) {
	t.Run("returns false when no user is set", func(t *testing.T) {
		sess := &Session{}

		if got := sess.IsAuthenticated(); got {
			t.Error("want: false; got: true")
		}
	})

	t.Run("returns true when a user is set", func(t *testing.T) {
		sess := &Session{user: &User{Email: "alice@example.com"}}

		if got := sess.IsAuthenticated(); !got {
			t.Error("want: true; got: false")
		}
	})
}

func TestSession_Get(t *testing.T) {
	t.Run("returns (nil, false) on absent key", func(t *testing.T) {
		sess := &Session{}

		value, ok := sess.Get("missing")

		if ok {
			t.Error("want ok: false; got: true")
		}
		if value != nil {
			t.Errorf("want: nil; got: %v", value)
		}
	})

	t.Run("returns the stored value", func(t *testing.T) {
		sess := &Session{data: map[string]any{"k": "v"}}

		value, ok := sess.Get("k")

		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})
}

func TestSession_Set(t *testing.T) {
	t.Run("stores the value under the key", func(t *testing.T) {
		sess := &Session{data: map[string]any{}}

		sess.Set("k", "v")

		value, ok := sess.data["k"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})

	t.Run("lazily initialises the data map", func(t *testing.T) {
		sess := &Session{}

		sess.Set("k", "v")

		if got := sess.data; got == nil {
			t.Fatal("want: non-nil; got: nil")
		}

		value, ok := sess.data["k"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})

	t.Run("overwrites an existing key", func(t *testing.T) {
		sess := &Session{data: map[string]any{"k": "v1"}}

		sess.Set("k", "v2")

		value, ok := sess.data["k"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v2"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})
}

func TestSession_Delete(t *testing.T) {
	t.Run("removes a stored key", func(t *testing.T) {
		sess := &Session{data: map[string]any{"k": "v"}}

		sess.Delete("k")

		if _, ok := sess.data["k"]; ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("is a no-op on unknown key", func(t *testing.T) {
		sess := &Session{data: map[string]any{"other": "v"}}

		sess.Delete("missing")

		if _, ok := sess.data["missing"]; ok {
			t.Error("want ok: false; got: true")
		}
	})
}

func TestSession_SetUser(t *testing.T) {
	t.Run("rotates ID and CSRF token on unauth->auth", func(t *testing.T) {
		sess := &Session{id: "id-1", csrfToken: "csrf-1"}

		sess.SetUser(&User{Email: "alice@example.com"})

		if want, got := "id-1", sess.id; want == got {
			t.Errorf("want: != %q; got: %q", want, got)
		}
		if want, got := "csrf-1", sess.csrfToken; want == got {
			t.Errorf("want: != %q; got: %q", want, got)
		}
	})

	t.Run("does not rotate on auth->auth", func(t *testing.T) {
		sess := &Session{
			id:        "id-1",
			csrfToken: "csrf-1",
			user:      &User{Email: "alice@example.com"},
		}

		sess.SetUser(&User{Email: "bob@example.com"})

		if want, got := "id-1", sess.id; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "csrf-1", sess.csrfToken; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}

func TestSession_Rotate(t *testing.T) {
	t.Run("changes ID and CSRF token", func(t *testing.T) {
		sess := &Session{id: "id-1", csrfToken: "csrf-1"}

		sess.Rotate()

		if want, got := "id-1", sess.id; want == got {
			t.Errorf("want: != %q; got: %q", want, got)
		}
		if want, got := "csrf-1", sess.csrfToken; want == got {
			t.Errorf("want: != %q; got: %q", want, got)
		}
	})

	t.Run("preserves user and data", func(t *testing.T) {
		user := &User{Email: "alice@example.com"}
		sess := &Session{
			id:        "id-1",
			csrfToken: "csrf-1",
			user:      user,
			data:      map[string]any{"k": "v"},
		}

		sess.Rotate()

		if got := sess.user; user != got {
			t.Errorf("want: %+v; got: %+v", user, got)
		}

		value, ok := sess.data["k"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := "v"; want != value {
			t.Errorf("want: %v; got: %v", want, value)
		}
	})
}
