package memstore

import (
	"context"
	"errors"
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
	t.Run("holds at most 50,000 entries by default", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		for i := range 50_000 {
			if err := store.Commit(t.Context(), "id-"+strconv.Itoa(i), []byte("payload"), time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Millisecond)
		}
		before, err := store.Prepare(t.Context(), "id-0")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if before == nil {
			t.Fatal("want before: non-nil; got: nil")
		}

		err = store.Commit(t.Context(), "id-50000", []byte("payload"), time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		after, err := store.Prepare(t.Context(), "id-0")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if after != nil {
			t.Error("want after: nil; got: non-nil")
		}
	})

	t.Run("holds at most 64 MiB by default", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		data := make([]byte, 1<<20)
		for i := range 64 {
			if err := store.Commit(t.Context(), "id-"+strconv.Itoa(i), data, time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Millisecond)
		}
		before, err := store.Prepare(t.Context(), "id-0")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if before == nil {
			t.Fatal("want before: non-nil; got: nil")
		}

		err = store.Commit(t.Context(), "id-64", data, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		after, err := store.Prepare(t.Context(), "id-0")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if after != nil {
			t.Error("want after: nil; got: non-nil")
		}
	})
}

func TestWithClock(t *testing.T) {
	t.Run("uses the given clock to evaluate TTLs", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		now = now.Add(time.Minute)
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})
}

func TestWithMaxEntries(t *testing.T) {
	t.Run("caps how many entries the store holds", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }), WithMaxEntries(2))
		for _, id := range []string{"id-1", "id-2"} {
			if err := store.Commit(t.Context(), id, []byte("payload"), time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Second)
		}

		err := store.Commit(t.Context(), "id-3", []byte("payload"), time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		first, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if first != nil {
			t.Error("want first: nil; got: non-nil")
		}
	})

	t.Run("keeps the default", func(t *testing.T) {
		for _, test := range []struct {
			name       string
			maxEntries int
		}{
			{name: "for zero", maxEntries: 0},
			{name: "for a negative value", maxEntries: -1},
		} {
			t.Run(test.name, func(t *testing.T) {
				now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				store := New(WithClock(func() time.Time { return now }), WithMaxEntries(test.maxEntries))
				for i := range 50_000 {
					if err := store.Commit(t.Context(), "id-"+strconv.Itoa(i), []byte("payload"), time.Hour); err != nil {
						t.Fatalf("store.Commit: %v", err)
					}
					now = now.Add(time.Millisecond)
				}
				before, err := store.Prepare(t.Context(), "id-0")
				if err != nil {
					t.Fatalf("store.Prepare: %v", err)
				}
				if before == nil {
					t.Fatal("want before: non-nil; got: nil")
				}

				err = store.Commit(t.Context(), "id-50000", []byte("payload"), time.Hour)

				if err != nil {
					t.Fatalf("want err: nil; got: %v", err)
				}
				after, err := store.Prepare(t.Context(), "id-0")
				if err != nil {
					t.Fatalf("store.Prepare: %v", err)
				}
				if after != nil {
					t.Error("want after: nil; got: non-nil")
				}
			})
		}
	})
}

func TestWithMaxBytes(t *testing.T) {
	t.Run("caps the total size of the data the store holds", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		data := []byte("payload")
		store := New(WithClock(func() time.Time { return now }), WithMaxBytes(2*len(data)))
		for _, id := range []string{"id-1", "id-2"} {
			if err := store.Commit(t.Context(), id, data, time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Second)
		}

		err := store.Commit(t.Context(), "id-3", data, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		first, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if first != nil {
			t.Error("want first: nil; got: non-nil")
		}
	})

	t.Run("keeps the default", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			maxBytes int
		}{
			{name: "for zero", maxBytes: 0},
			{name: "for a negative value", maxBytes: -1},
		} {
			t.Run(test.name, func(t *testing.T) {
				now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				store := New(WithClock(func() time.Time { return now }), WithMaxBytes(test.maxBytes))
				data := make([]byte, 1<<20)
				for i := range 64 {
					if err := store.Commit(t.Context(), "id-"+strconv.Itoa(i), data, time.Hour); err != nil {
						t.Fatalf("store.Commit: %v", err)
					}
					now = now.Add(time.Millisecond)
				}
				before, err := store.Prepare(t.Context(), "id-0")
				if err != nil {
					t.Fatalf("store.Prepare: %v", err)
				}
				if before == nil {
					t.Fatal("want before: non-nil; got: nil")
				}

				err = store.Commit(t.Context(), "id-64", data, time.Hour)

				if err != nil {
					t.Fatalf("want err: nil; got: %v", err)
				}
				after, err := store.Prepare(t.Context(), "id-0")
				if err != nil {
					t.Fatalf("store.Prepare: %v", err)
				}
				if after != nil {
					t.Error("want after: nil; got: non-nil")
				}
			})
		}
	})
}

func TestStore_Prepare(t *testing.T) {
	t.Run("returns (nil, nil) for an ID with no entry", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		data, err := store.Prepare(t.Context(), "missing")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("returns (nil, nil) for an empty ID", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		data, err := store.Prepare(t.Context(), "")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("returns the stored bytes", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		data, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("returns (nil, nil) once the entry has expired", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		// An entry is already expired at its expiry instant, not only after it.
		now = now.Add(time.Minute)

		data, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("frees the entry once it has expired", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		now = now.Add(time.Minute)

		_, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries["id-1"]; ok {
			t.Error("want store.entries[\"id-1\"] ok: false; got: true")
		}
		if got := store.bytes; got != 0 {
			t.Errorf("want store.bytes: 0; got: %d", got)
		}
	})

	t.Run("returns an independent copy to each caller", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		first, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if len(first) == 0 {
			t.Fatal("want first: non-empty; got: empty")
		}

		// Mutating one caller's bytes must reach neither the store nor a later
		// caller.
		first[0] = 'X'

		second, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "payload", string(second); want != got {
			t.Errorf("want second: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the context is done", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		data, err := store.Prepare(ctx, "id-1")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})
}

func TestStore_Commit(t *testing.T) {
	t.Run("stores the data for the TTL", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))

		err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		now = now.Add(time.Minute - time.Nanosecond)
		beforeExpiry, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "payload", string(beforeExpiry); want != got {
			t.Errorf("want beforeExpiry: %q; got: %q", want, got)
		}

		now = now.Add(time.Nanosecond)
		atExpiry, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if atExpiry != nil {
			t.Error("want atExpiry: nil; got: non-nil")
		}
	})

	t.Run("stores an independent copy of the caller's bytes", func(t *testing.T) {
		store := New()
		payload := []byte("payload")

		err := store.Commit(t.Context(), "id-1", payload, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		// Committed state is fixed at the call, so this lands nowhere.
		payload[0] = 'X'

		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("replaces an existing entry", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if err := store.Commit(t.Context(), "id-1", []byte("old"), time.Second); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		err := store.Commit(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		// Past the old entry's TTL, so the entry survives only if its TTL was replaced.
		now = now.Add(time.Second)
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "new", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("evicts expired entries before live ones when the store is full", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }), WithMaxEntries(2))
		// The live entry is the least recently committed, so only the expiry can
		// make the store pick the other one.
		if err := store.Commit(t.Context(), "live", []byte("payload"), time.Hour); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}
		now = now.Add(time.Second)
		if err := store.Commit(t.Context(), "expired", []byte("payload"), time.Second); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}
		now = now.Add(time.Second)

		err := store.Commit(t.Context(), "id-3", []byte("payload"), time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		live, err := store.Prepare(t.Context(), "live")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if live == nil {
			t.Error("want live: non-nil; got: nil")
		}
	})

	t.Run("evicts the least recently committed entry when the store is full", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }), WithMaxEntries(2))
		// id-1 is committed first but again after id-2, so evicting by first
		// commit would pick the wrong one.
		for _, id := range []string{"id-1", "id-2", "id-1"} {
			if err := store.Commit(t.Context(), id, []byte("payload"), time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Second)
		}

		err := store.Commit(t.Context(), "id-3", []byte("payload"), time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		idle, err := store.Prepare(t.Context(), "id-2")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if idle != nil {
			t.Error("want idle: nil; got: non-nil")
		}
		recent, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if recent == nil {
			t.Error("want recent: non-nil; got: nil")
		}
	})

	t.Run("evicts entries to stay within the byte limit", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		data := []byte("payload")
		// The entry limit is far off, so only the byte limit can force the
		// eviction.
		store := New(WithClock(func() time.Time { return now }), WithMaxEntries(10), WithMaxBytes(2*len(data)))
		for _, id := range []string{"id-1", "id-2"} {
			if err := store.Commit(t.Context(), id, data, time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Second)
		}

		err := store.Commit(t.Context(), "id-3", data, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		first, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if first != nil {
			t.Error("want first: nil; got: non-nil")
		}
	})

	t.Run("keeps other entries when an entry is replaced at the limits", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		data := []byte("payload")
		// Full on both limits, so counting the replacement as an extra entry or
		// as extra bytes would each force an eviction.
		store := New(WithClock(func() time.Time { return now }), WithMaxEntries(2), WithMaxBytes(2*len(data)))
		for _, id := range []string{"id-1", "id-2"} {
			if err := store.Commit(t.Context(), id, data, time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Second)
		}

		err := store.Commit(t.Context(), "id-2", []byte("PAYLOAD"), time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		other, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if other == nil {
			t.Error("want other: non-nil; got: nil")
		}
	})

	t.Run("rejects an empty ID", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		err := store.Commit(t.Context(), "", []byte("new"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "id must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		if _, ok := store.entries[""]; ok {
			t.Error("want store.entries[\"\"] ok: false; got: true")
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects a TTL", func(t *testing.T) {
		for _, test := range []struct {
			name string
			ttl  time.Duration
		}{
			{name: "of zero", ttl: 0},
			{name: "below zero", ttl: -time.Second},
		} {
			t.Run(test.name, func(t *testing.T) {
				store := New()
				if err := store.Commit(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
					t.Fatalf("store.Commit: %v", err)
				}

				err := store.Commit(t.Context(), "id-1", []byte("new"), test.ttl)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if want := "ttl must be positive"; !strings.Contains(err.Error(), want) {
					t.Errorf("want err: containing %q; got: %q", want, err)
				}
				data, err := store.Prepare(t.Context(), "id-1")
				if err != nil {
					t.Fatalf("store.Prepare: %v", err)
				}
				if want, got := "old", string(data); want != got {
					t.Errorf("want data: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("rejects data larger than the byte limit", func(t *testing.T) {
		store := New(WithMaxBytes(len("payload")))

		err := store.Commit(t.Context(), "id-1", []byte("payload!"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("leaves the entry unchanged when its replacement is larger than the byte limit", func(t *testing.T) {
		store := New(WithMaxBytes(len("payload")))
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		err := store.Commit(t.Context(), "id-1", []byte("payload!"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the context is done", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Commit(ctx, "id-1", []byte("new"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})
}

func TestStore_Drop(t *testing.T) {
	t.Run("removes a stored entry", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		err := store.Drop(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("frees the dropped entry's bytes for later commits", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		data := []byte("payload")
		store := New(WithClock(func() time.Time { return now }), WithMaxEntries(10), WithMaxBytes(2*len(data)))
		for _, id := range []string{"id-1", "id-2"} {
			if err := store.Commit(t.Context(), id, data, time.Hour); err != nil {
				t.Fatalf("store.Commit: %v", err)
			}
			now = now.Add(time.Second)
		}
		if err := store.Drop(t.Context(), "id-1"); err != nil {
			t.Fatalf("store.Drop: %v", err)
		}

		err := store.Commit(t.Context(), "id-3", data, time.Hour)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		kept, err := store.Prepare(t.Context(), "id-2")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if kept == nil {
			t.Error("want kept: non-nil; got: nil")
		}
	})

	t.Run("is a no-op for an ID with no entry", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		err := store.Drop(t.Context(), "missing")

		if err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("is a no-op for an empty ID", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		err := store.Drop(t.Context(), "")

		if err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the context is done", func(t *testing.T) {
		store := New()
		if err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Commit: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Drop(ctx, "id-1")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Prepare: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})
}

func TestStore(t *testing.T) {
	t.Run("is safe for concurrent use", func(t *testing.T) {
		// The primary signal comes from `go test -race`, which flags any
		// unsynchronised access to the entries map regardless of scheduling.
		store := New()

		const groups = 4
		const iterations = 20
		ids := []string{"id-0", "id-1"}

		var wg sync.WaitGroup
		wg.Add(groups * 3)

		for g := range groups {
			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]
					data := []byte(id + ":" + strconv.Itoa(g))

					err := store.Commit(t.Context(), id, data, time.Minute)

					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
				}
			}()

			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]

					data, err := store.Prepare(t.Context(), id)

					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
					if data == nil {
						continue
					}
					if want, got := id+":", string(data); !strings.HasPrefix(got, want) {
						t.Errorf("want data: prefix %q; got: %q", want, got)
						return
					}
				}
			}()

			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]

					err := store.Drop(t.Context(), id)

					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
				}
			}()
		}

		wg.Wait()
	})

	t.Run("stays within its entry limit under concurrent use", func(t *testing.T) {
		// A limit this small has nearly every commit evicting, so the eviction
		// path runs under contention, where `go test -race` can see it.
		const (
			maxEntries = 8
			groups     = 8
			iterations = 50
		)

		store := New(WithMaxEntries(maxEntries))

		var wg sync.WaitGroup
		wg.Add(groups)

		for g := range groups {
			go func() {
				defer wg.Done()

				for i := range iterations {
					id := "id-" + strconv.Itoa(g) + "-" + strconv.Itoa(i)

					err := store.Commit(t.Context(), id, []byte("payload"), time.Minute)

					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
				}
			}()
		}

		wg.Wait()

		var held int
		for g := range groups {
			for i := range iterations {
				data, err := store.Prepare(t.Context(), "id-"+strconv.Itoa(g)+"-"+strconv.Itoa(i))
				if err != nil {
					t.Fatalf("store.Prepare: %v", err)
				}
				if data != nil {
					held++
				}
			}
		}
		if held > maxEntries {
			t.Errorf("want held: <= %d; got: %d", maxEntries, held)
		}

		// No API reports the byte total the limit is enforced against, so check
		// it still matches the data the store holds.
		var want int
		for _, e := range store.entries {
			want += len(e.data)
		}
		if got := store.bytes; want != got {
			t.Errorf("want store.bytes: %d; got: %d", want, got)
		}
	})
}
