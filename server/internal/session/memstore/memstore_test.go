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

	t.Run("defaults the limits", func(t *testing.T) {
		store := New()

		if want, got := defaultMaxEntries, store.maxEntries; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := defaultMaxBytes, store.maxBytes; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("applies the configured limits", func(t *testing.T) {
		store := New(WithMaxEntries(7), WithMaxBytes(2048))

		if want, got := 7, store.maxEntries; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := 2048, store.maxBytes; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("keeps the defaults for limits below 1", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			limit int
		}{
			{name: "zero", limit: 0},
			{name: "negative", limit: -1},
		} {
			t.Run(tt.name, func(t *testing.T) {
				store := New(WithMaxEntries(tt.limit), WithMaxBytes(tt.limit))

				if want, got := defaultMaxEntries, store.maxEntries; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := defaultMaxBytes, store.maxBytes; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
			})
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

	t.Run("records whether the snapshot carried a user", func(t *testing.T) {
		// The flag decides what the store gives up first, so it has to follow
		// the snapshot rather than the order entries arrived in.
		for _, tt := range []struct {
			name string
			user *session.User
			want bool
		}{
			{name: "with a user", user: &session.User{Email: "alice@example.com"}, want: true},
			{name: "without a user", user: nil, want: false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				store := New()
				snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1", User: tt.user}

				if err := store.Commit(context.Background(), snapshot, time.Minute); err != nil {
					t.Fatalf("want err: nil; got: %v", err)
				}

				if got := store.entries["id-1"].authenticated; tt.want != got {
					t.Errorf("want: %v; got: %v", tt.want, got)
				}
			})
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

// seedEntry stores an entry directly and keeps the byte total in step, so a
// test can arrange a full store without going through Commit, whose eviction
// is what these tests exercise.
func seedEntry(s *Store, id string, e entry) {
	s.entries[id] = e
	s.bytes += len(e.data)
}

func TestStore_Limits(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Every snapshot below encodes to 35 bytes, so the byte cases can count in
	// whole entries.
	const entryBytes = 35

	newStore := func(maxEntries, maxBytes int) *Store {
		return New(
			WithClock(func() time.Time { return now }),
			WithMaxEntries(maxEntries),
			WithMaxBytes(maxBytes),
		)
	}

	t.Run("drops expired entries before live ones", func(t *testing.T) {
		store := newStore(2, 1<<20)
		seedEntry(store, "expired", entry{
			data:        []byte(`{"id":"expired","csrf_token":"csrf-1"}`),
			expiresAt:   now.Add(-time.Second),
			committedAt: now.Add(-time.Hour),
		})
		seedEntry(store, "live", entry{
			data:        []byte(`{"id":"live","csrf_token":"csrf-2"}`),
			expiresAt:   now.Add(time.Hour),
			committedAt: now.Add(-time.Minute),
		})

		err := store.Commit(context.Background(), &session.Snapshot{ID: "id-3", CSRFToken: "csrf-3"}, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries["expired"]; ok {
			t.Error("want ok: false; got: true")
		}
		if _, ok := store.entries["live"]; !ok {
			t.Error("want ok: true; got: false")
		}
		if _, ok := store.entries["id-3"]; !ok {
			t.Error("want ok: true; got: false")
		}
	})

	t.Run("evicts the least recently committed unauthenticated entry", func(t *testing.T) {
		store := newStore(2, 1<<20)
		seedEntry(store, "idle", entry{
			data:        []byte(`{"id":"idle","csrf_token":"csrf-1"}`),
			expiresAt:   now.Add(time.Hour),
			committedAt: now.Add(-time.Hour),
		})
		seedEntry(store, "recent", entry{
			data:        []byte(`{"id":"recent","csrf_token":"csrf-2"}`),
			expiresAt:   now.Add(time.Hour),
			committedAt: now.Add(-time.Minute),
		})

		err := store.Commit(context.Background(), &session.Snapshot{ID: "id-3", CSRFToken: "csrf-3"}, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries["idle"]; ok {
			t.Error("want ok: false; got: true")
		}
		if _, ok := store.entries["recent"]; !ok {
			t.Error("want ok: true; got: false")
		}
	})

	t.Run("keeps authenticated entries while unauthenticated ones remain", func(t *testing.T) {
		// The authenticated entry is the older of the two, so recency alone
		// would have taken it.
		store := newStore(2, 1<<20)
		seedEntry(store, "signed-in", entry{
			data:          []byte(`{"id":"signed-in","csrf_token":"csrf-1"}`),
			expiresAt:     now.Add(time.Hour),
			committedAt:   now.Add(-time.Hour),
			authenticated: true,
		})
		seedEntry(store, "signed-out", entry{
			data:        []byte(`{"id":"signed-out","csrf_token":"csrf-2"}`),
			expiresAt:   now.Add(time.Hour),
			committedAt: now.Add(-time.Minute),
		})

		err := store.Commit(context.Background(), &session.Snapshot{ID: "id-3", CSRFToken: "csrf-3"}, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries["signed-in"]; !ok {
			t.Error("want ok: true; got: false")
		}
		if _, ok := store.entries["signed-out"]; ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("rejects a new session when only authenticated ones remain", func(t *testing.T) {
		store := newStore(1, 1<<20)
		seedEntry(store, "signed-in", entry{
			data:          []byte(`{"id":"signed-in","csrf_token":"csrf-1"}`),
			expiresAt:     now.Add(time.Hour),
			committedAt:   now.Add(-time.Hour),
			authenticated: true,
		})

		err := store.Commit(context.Background(), &session.Snapshot{ID: "id-2", CSRFToken: "csrf-2"}, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "store is full"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		if _, ok := store.entries["id-2"]; ok {
			t.Error("want ok: false; got: true")
		}
		if _, ok := store.entries["signed-in"]; !ok {
			t.Error("want ok: true; got: false")
		}
	})

	t.Run("replaces an existing entry at the limit without evicting", func(t *testing.T) {
		store := newStore(1, 1<<20)
		seedEntry(store, "id-1", entry{
			data:          []byte(`{"id":"id-1","csrf_token":"csrf-1"}`),
			expiresAt:     now.Add(time.Hour),
			committedAt:   now.Add(-time.Hour),
			authenticated: true,
		})

		err := store.Commit(context.Background(), &session.Snapshot{ID: "id-1", CSRFToken: "csrf-2"}, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := `{"id":"id-1","csrf_token":"csrf-2"}`, string(store.entries["id-1"].data); want != got {
			t.Errorf("want: %s; got: %s", want, got)
		}
		if want, got := 1, len(store.entries); want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("evicts to stay within the byte limit", func(t *testing.T) {
		// Room for two entries, so the third has to displace one even though
		// the entry limit is far off.
		store := newStore(10, entryBytes*2)
		seedEntry(store, "idle", entry{
			data:        []byte(`{"id":"idle","csrf_token":"csr-1"}`),
			expiresAt:   now.Add(time.Hour),
			committedAt: now.Add(-time.Hour),
		})
		seedEntry(store, "recent", entry{
			data:        []byte(`{"id":"recnt","csrf_token":"csr-2"}`),
			expiresAt:   now.Add(time.Hour),
			committedAt: now.Add(-time.Minute),
		})

		err := store.Commit(context.Background(), &session.Snapshot{ID: "id-3", CSRFToken: "csrf-3"}, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries["idle"]; ok {
			t.Error("want ok: false; got: true")
		}
		if want, got := entryBytes*2, store.bytes; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("keeps the byte total in step as entries come and go", func(t *testing.T) {
		store := newStore(10, 1<<20)
		ctx := context.Background()

		if err := store.Commit(ctx, &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := entryBytes, store.bytes; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}

		if err := store.Drop(ctx, "id-1"); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if got := store.bytes; got != 0 {
			t.Errorf("want: 0; got: %d", got)
		}
	})

	t.Run("releases bytes when an expired entry is read", func(t *testing.T) {
		clock := now
		store := New(
			WithClock(func() time.Time { return clock }),
			WithMaxEntries(10),
			WithMaxBytes(1<<20),
		)
		seedEntry(store, "id-1", entry{
			data:        []byte(`{"id":"id-1","csrf_token":"csrf-1"}`),
			expiresAt:   now.Add(time.Minute),
			committedAt: now,
		})

		clock = now.Add(time.Hour)

		if _, err := store.Prepare(context.Background(), "id-1"); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if got := store.bytes; got != 0 {
			t.Errorf("want: 0; got: %d", got)
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

	t.Run("holds its limits while evicting under load", func(t *testing.T) {
		// A cap this small has nearly every commit culling, so the eviction
		// path runs under contention, where `go test -race` can see it.
		const (
			maxEntries = 8
			goroutines = 8
			iterations = 50
		)

		store := New(WithMaxEntries(maxEntries), WithMaxBytes(1<<20))

		var wg sync.WaitGroup
		wg.Add(goroutines)

		for g := range goroutines {
			go func() {
				defer wg.Done()

				for i := range iterations {
					id := "id-" + strconv.Itoa(g) + "-" + strconv.Itoa(i)

					if err := store.Commit(context.Background(), &session.Snapshot{ID: id, CSRFToken: "csrf-1"}, time.Minute); err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
					if _, err := store.Prepare(context.Background(), id); err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
				}
			}()
		}

		wg.Wait()

		if got := len(store.entries); got > maxEntries {
			t.Errorf("want: at most %d; got: %d", maxEntries, got)
		}

		// The byte total is kept by hand on every path that adds or removes an
		// entry, so it has to still match what the store holds.
		var want int
		for _, e := range store.entries {
			want += len(e.data)
		}
		if got := store.bytes; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
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
