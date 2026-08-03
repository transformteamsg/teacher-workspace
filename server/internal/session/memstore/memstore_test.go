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
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1","user":{"email":"alice@example.com"},"data":{"k":"v"}}`),
			expiresAt: store.now().Add(time.Minute),
		}

		result, err := store.Prepare(context.Background(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result == nil {
			t.Fatal("want: non-nil; got: nil")
		}
		if want, got := "id-1", result.ID; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "csrf-1", result.CSRFToken; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if result.User == nil {
			t.Fatal("want user: non-nil; got: nil")
		}
		if want, got := "alice@example.com", result.User.Email; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "v", result.Data["k"]; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})

	t.Run("returns an error on an undecodable entry", func(t *testing.T) {
		store := New()
		store.entries["id-1"] = entry{
			data:      []byte("not json"),
			expiresAt: store.now().Add(time.Minute),
		}

		result, err := store.Prepare(context.Background(), "id-1")

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "unmarshal snapshot"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns an independent snapshot to each caller", func(t *testing.T) {
		store := New()
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1","data":{"k":"v"}}`),
			expiresAt: store.now().Add(time.Minute),
		}

		first, err := store.Prepare(context.Background(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		// Mutating one caller's snapshot must reach neither the store nor a
		// later caller.
		first.Data["k"] = "mutated"

		second, err := store.Prepare(context.Background(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "v", second.Data["k"]; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})
}

func TestStore_TTL(t *testing.T) {
	t.Run("expires entry after TTL elapses", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1"}`),
			expiresAt: now.Add(time.Minute),
		}

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
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1"}`),
			expiresAt: now.Add(time.Minute),
		}

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
		if want, got := `{"id":"id-1","csrf_token":"csrf-1"}`, string(ent.data); want != got {
			t.Errorf("want: %s; got: %s", want, got)
		}
		if want, got := now.Add(time.Minute), ent.expiresAt; !want.Equal(got) {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})

	t.Run("stores a copy the caller cannot reach", func(t *testing.T) {
		store := New()
		snapshot := &session.Snapshot{
			ID:        "id-1",
			CSRFToken: "csrf-1",
			Data:      map[string]any{"k": "v"},
		}

		if err := store.Commit(context.Background(), snapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		// Committed state is fixed at the call, so this lands nowhere.
		snapshot.Data["k"] = "mutated"

		result, err := store.Prepare(context.Background(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "v", result.Data["k"]; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects data that cannot be encoded", func(t *testing.T) {
		store := New()
		snapshot := &session.Snapshot{
			ID:        "id-1",
			CSRFToken: "csrf-1",
			Data:      map[string]any{"ch": make(chan int)},
		}

		err := store.Commit(context.Background(), snapshot, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "marshal snapshot"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		if _, ok := store.entries["id-1"]; ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("overwrites an existing entry", func(t *testing.T) {
		store := New()
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1"}`),
			expiresAt: store.now().Add(time.Minute),
		}

		newSnapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-2"}
		if err := store.Commit(context.Background(), newSnapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		if want, got := `{"id":"id-1","csrf_token":"csrf-2"}`, string(store.entries["id-1"].data); want != got {
			t.Errorf("want: %s; got: %s", want, got)
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

func TestStore_RoundTrip(t *testing.T) {
	t.Run("returns data in its JSON form", func(t *testing.T) {
		// Values come back shaped by the encoding, not as the Go types that
		// went in, so callers must assert against the decoded form.
		store := New()
		snapshot := &session.Snapshot{
			ID:        "id-1",
			CSRFToken: "csrf-1",
			Data:      map[string]any{"count": 1},
		}

		if err := store.Commit(context.Background(), snapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		result, err := store.Prepare(context.Background(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		if want, got := float64(1), result.Data["count"]; want != got {
			t.Errorf("want: %[1]v (%[1]T); got: %[2]v (%[2]T)", want, got)
		}
	})
}

func TestStore_Drop(t *testing.T) {
	t.Run("removes a stored entry", func(t *testing.T) {
		store := New()
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1"}`),
			expiresAt: store.now().Add(time.Minute),
		}

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
	t.Run("snapshots handed to concurrent callers are independent", func(t *testing.T) {
		// Two requests carrying the same session cookie each prepare the entry
		// and write to their own session. Handing both the same Data map makes
		// that a concurrent map write, which `go test -race` flags.
		store := New()
		store.entries["id-1"] = entry{
			data:      []byte(`{"id":"id-1","csrf_token":"csrf-1","data":{"k":"v"}}`),
			expiresAt: store.now().Add(time.Minute),
		}

		const goroutines = 8

		var wg sync.WaitGroup
		wg.Add(goroutines)

		for g := range goroutines {
			go func() {
				defer wg.Done()

				prepared, err := store.Prepare(context.Background(), "id-1")
				if err != nil {
					t.Errorf("want err: nil; got: %v", err)
					return
				}
				if prepared == nil {
					t.Error("want: non-nil; got: nil")
					return
				}

				session.FromSnapshot(prepared).Set("g", g)
			}()
		}

		wg.Wait()
	})

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
