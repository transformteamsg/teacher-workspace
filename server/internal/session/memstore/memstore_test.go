package memstore

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// Asserts at compile time that *Store satisfies session.Store.
var _ session.Store = (*Store)(nil)

func TestNew(t *testing.T) {
	t.Run("initialises entries and clock", func(t *testing.T) {
		store := New()

		if got := store.entries; got == nil {
			t.Error("want: non-nil; got: nil")
		}
		if got := store.now; got == nil {
			t.Error("want: non-nil; got: nil")
		}
	})

	t.Run("applies options", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))

		if got := store.now(); !now.Equal(got) {
			t.Errorf("want: %v; got: %v", now, got)
		}
	})
}

func TestStore_Prepare(t *testing.T) {
	t.Run("returns (nil, nil) for unknown id", func(t *testing.T) {
		store := New()

		result, err := store.Prepare(context.Background(), "missing")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns (nil, nil) for empty id", func(t *testing.T) {
		store := New()

		result, err := store.Prepare(context.Background(), "")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns the stored snapshot", func(t *testing.T) {
		store := New()
		snapshot := &session.Snapshot{
			ID:        "id-1",
			CSRFToken: "csrf-1",
			User:      &session.User{Email: "alice@example.com"},
			Data:      map[string]any{"k": "v"},
		}
		store.entries[snapshot.ID] = entry{snap: snapshot, expiresAt: store.now().Add(time.Minute)}

		result, err := store.Prepare(context.Background(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if snapshot != result {
			t.Errorf("want: %+v; got: %+v", snapshot, result)
		}
	})
}

func TestStore_TTL(t *testing.T) {
	t.Run("expires entry after TTL elapses", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}
		store.entries["id-1"] = entry{snap: snapshot, expiresAt: now.Add(time.Minute)}

		now = now.Add(time.Minute + time.Nanosecond)

		result, err := store.Prepare(context.Background(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns the entry while still within TTL", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}
		store.entries["id-1"] = entry{snap: snapshot, expiresAt: now.Add(time.Minute)}

		now = now.Add(30 * time.Second)

		result, err := store.Prepare(context.Background(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result == nil {
			t.Fatal("want: non-nil; got: nil")
		}
	})
}

func TestStore_Commit(t *testing.T) {
	t.Run("stores snapshot under its ID with computed expiry", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}

		if err := store.Commit(context.Background(), snapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		ent, ok := store.entries["id-1"]
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if got := ent.snap; snapshot != got {
			t.Errorf("want: %+v; got: %+v", snapshot, got)
		}
		if want, got := now.Add(time.Minute), ent.expiresAt; !want.Equal(got) {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})

	t.Run("overwrites an existing entry", func(t *testing.T) {
		store := New()
		oldSnapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}
		newSnapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-2"}
		store.entries["id-1"] = entry{snap: oldSnapshot, expiresAt: store.now().Add(time.Minute)}

		if err := store.Commit(context.Background(), newSnapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		if got := store.entries["id-1"].snap; newSnapshot != got {
			t.Errorf("want: %+v; got: %+v", newSnapshot, got)
		}
	})

	t.Run("rejects nil snapshot", func(t *testing.T) {
		store := New()

		err := store.Commit(context.Background(), nil, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "snap must be non-nil"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("rejects empty snap.ID", func(t *testing.T) {
		store := New()

		err := store.Commit(context.Background(), &session.Snapshot{}, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "snap.ID must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("rejects non-positive TTL", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			ttl  time.Duration
		}{
			{name: "zero", ttl: 0},
			{name: "negative", ttl: -time.Second},
		} {
			t.Run(tt.name, func(t *testing.T) {
				store := New()

				err := store.Commit(context.Background(), &session.Snapshot{ID: "id-1"}, tt.ttl)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if want := "ttl must be positive"; !strings.Contains(err.Error(), want) {
					t.Errorf("want err: containing %q; got: %q", want, err)
				}
			})
		}
	})
}

func TestStore_Drop(t *testing.T) {
	t.Run("removes a stored entry", func(t *testing.T) {
		store := New()
		snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}
		store.entries["id-1"] = entry{snap: snapshot, expiresAt: store.now().Add(time.Minute)}

		if err := store.Drop(context.Background(), "id-1"); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		if _, ok := store.entries["id-1"]; ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("is a no-op on an unknown id", func(t *testing.T) {
		store := New()

		if err := store.Drop(context.Background(), "missing"); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("is a no-op on empty id", func(t *testing.T) {
		store := New()

		if err := store.Drop(context.Background(), ""); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})
}

func TestStore_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent Commit and Prepare on shared keys stay consistent", func(t *testing.T) {
		// The primary signal comes from `go test -race`, which flags any
		// unsynchronised access to the entries map regardless of scheduling.
		store := New()

		const pairs = 4
		const iterations = 20
		ids := []string{"id-0", "id-1"}

		var wg sync.WaitGroup
		wg.Add(pairs * 2)

		for g := range pairs {
			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]
					snapshot := &session.Snapshot{ID: id, CSRFToken: strconv.Itoa(g)}
					if err := store.Commit(context.Background(), snapshot, time.Minute); err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
				}
			}()

			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]
					snapshot, err := store.Prepare(context.Background(), id)
					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
					if snapshot == nil {
						continue
					}
					if got := snapshot.ID; id != got {
						t.Errorf("want: %q; got: %q", id, got)
						return
					}
				}
			}()
		}

		wg.Wait()
	})
}
