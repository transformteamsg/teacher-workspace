package memstore

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transformteamsg/teacher-workspace/server/internal/session"
)

// Asserts at compile time that *Store satisfies session.Store.
var _ session.Store = (*Store)(nil)

func TestWithClock(t *testing.T) {
	t.Run("uses the given clock to evaluate TTLs", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		now = now.Add(time.Minute)
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})
}

func TestStore_Get(t *testing.T) {
	t.Run("returns (nil, nil) when the ID has no entry", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		data, err := store.Get(t.Context(), "missing")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("returns (nil, nil) when the ID is empty", func(t *testing.T) {
		store := New()
		// Seeded directly, as Create never writes an empty ID, so it is read only if Get looks up an empty ID.
		store.entries[""] = entry{
			data:      []byte("payload"),
			expiresAt: time.Now().Add(time.Minute),
		}

		data, err := store.Get(t.Context(), "")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("returns the data stored under the ID", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		data, err := store.Get(t.Context(), "id-1")

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
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		// An entry is already expired at its expiry instant, not only after it.
		now = now.Add(time.Minute)

		data, err := store.Get(t.Context(), "id-1")

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
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		now = now.Add(time.Minute)

		_, err := store.Get(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries["id-1"]; ok {
			t.Error("want store.entries[\"id-1\"] ok: false; got: true")
		}
	})

	t.Run("returns an independent copy to each caller", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		first, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if len(first) == 0 {
			t.Fatal("want first: non-empty; got: empty")
		}

		// Mutating one caller's bytes must reach neither the store nor a later
		// caller.
		first[0] = 'X'

		second, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "payload", string(second); want != got {
			t.Errorf("want second: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Get(ctx, "id-1")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
	})

	t.Run("rejects the call when the ID is empty and the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Get(ctx, "")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
	})
}

func TestStore_Create(t *testing.T) {
	t.Run("stores data under the ID for the TTL", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))

		created, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if !created {
			t.Error("want created: true; got: false")
		}

		now = now.Add(time.Minute - time.Nanosecond)
		beforeExpiry, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "payload", string(beforeExpiry); want != got {
			t.Errorf("want beforeExpiry: %q; got: %q", want, got)
		}

		now = now.Add(time.Nanosecond)
		atExpiry, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if atExpiry != nil {
			t.Error("want atExpiry: nil; got: non-nil")
		}
	})

	t.Run("stores an independent copy of the caller's bytes", func(t *testing.T) {
		store := New()
		payload := []byte("payload")

		_, err := store.Create(t.Context(), "id-1", payload, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		// Stored state is fixed at the call, so this lands nowhere.
		payload[0] = 'X'

		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("does not replace an existing entry", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		created, err := store.Create(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if created {
			t.Error("want created: false; got: true")
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("stores data when the ID's entry has expired", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}
		now = now.Add(time.Minute)

		created, err := store.Create(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if !created {
			t.Error("want created: true; got: false")
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "new", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects an empty ID", func(t *testing.T) {
		store := New()

		_, err := store.Create(t.Context(), "", []byte("payload"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "id must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		if _, ok := store.entries[""]; ok {
			t.Error("want store.entries[\"\"] ok: false; got: true")
		}
	})

	t.Run("rejects empty data", func(t *testing.T) {
		store := New()

		_, err := store.Create(t.Context(), "id-1", []byte{}, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "data must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("rejects a TTL that is", func(t *testing.T) {
		for _, test := range []struct {
			name string
			ttl  time.Duration
		}{
			{
				name: "zero",
				ttl:  0,
			},
			{
				name: "negative",
				ttl:  -time.Second,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				store := New()

				_, err := store.Create(t.Context(), "id-1", []byte("payload"), test.ttl)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if want := "ttl must be positive"; !strings.Contains(err.Error(), want) {
					t.Errorf("want err: containing %q; got: %q", want, err)
				}
				data, err := store.Get(t.Context(), "id-1")
				if err != nil {
					t.Fatalf("store.Get: %v", err)
				}
				if data != nil {
					t.Error("want data: nil; got: non-nil")
				}
			})
		}
	})

	t.Run("rejects the call when the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Create(ctx, "id-1", []byte("payload"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("rejects the call when the ID is empty and the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Create(ctx, "", []byte("payload"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		if _, ok := store.entries[""]; ok {
			t.Error("want store.entries[\"\"] ok: false; got: true")
		}
	})

	t.Run("rejects the call when the data is empty and the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Create(ctx, "id-1", []byte{}, time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("rejects the call when the TTL is not positive and the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Create(ctx, "id-1", []byte("payload"), 0)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})
}

func TestStore_Update(t *testing.T) {
	t.Run("replaces an existing entry for the TTL", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Second); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		updated, err := store.Update(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if !updated {
			t.Error("want updated: true; got: false")
		}

		// Past the old entry's TTL, so the entry survives only if its TTL was replaced.
		now = now.Add(time.Minute - time.Nanosecond)
		beforeExpiry, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "new", string(beforeExpiry); want != got {
			t.Errorf("want beforeExpiry: %q; got: %q", want, got)
		}

		now = now.Add(time.Nanosecond)
		atExpiry, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if atExpiry != nil {
			t.Error("want atExpiry: nil; got: non-nil")
		}
	})

	t.Run("stores an independent copy of the caller's bytes", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}
		payload := []byte("new")

		_, err := store.Update(t.Context(), "id-1", payload, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		// Stored state is fixed at the call, so this lands nowhere.
		payload[0] = 'X'

		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "new", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("does not create an entry when the ID has no entry", func(t *testing.T) {
		store := New()

		updated, err := store.Update(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if updated {
			t.Error("want updated: false; got: true")
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("does not create an entry when the ID's entry has expired", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		store := New(WithClock(func() time.Time { return now }))
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}
		now = now.Add(time.Minute)

		updated, err := store.Update(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if updated {
			t.Error("want updated: false; got: true")
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("rejects an empty ID", func(t *testing.T) {
		store := New()

		_, err := store.Update(t.Context(), "", []byte("new"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "id must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("rejects empty data", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		_, err := store.Update(t.Context(), "id-1", []byte{}, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "data must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects a TTL that is", func(t *testing.T) {
		for _, test := range []struct {
			name string
			ttl  time.Duration
		}{
			{
				name: "zero",
				ttl:  0,
			},
			{
				name: "negative",
				ttl:  -time.Second,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				store := New()
				if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
					t.Fatalf("store.Create: %v", err)
				}

				_, err := store.Update(t.Context(), "id-1", []byte("new"), test.ttl)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if want := "ttl must be positive"; !strings.Contains(err.Error(), want) {
					t.Errorf("want err: containing %q; got: %q", want, err)
				}
				data, err := store.Get(t.Context(), "id-1")
				if err != nil {
					t.Fatalf("store.Get: %v", err)
				}
				if want, got := "old", string(data); want != got {
					t.Errorf("want data: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("rejects the call when the context is already done", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Update(ctx, "id-1", []byte("new"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the ID is empty and the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Update(ctx, "", []byte("new"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
	})

	t.Run("rejects the call when the data is empty and the context is already done", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Update(ctx, "id-1", []byte{}, time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the TTL is not positive and the context is already done", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("old"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Update(ctx, "id-1", []byte("new"), 0)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "old", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})
}

func TestStore_Delete(t *testing.T) {
	t.Run("removes the entry under the ID", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		err := store.Delete(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("is a no-op when the ID has no entry", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		err := store.Delete(t.Context(), "missing")

		if err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("is a no-op when the ID is empty", func(t *testing.T) {
		store := New()
		// Seeded directly, as Create never writes an empty ID, so it is deleted only if Delete looks up an empty ID.
		store.entries[""] = entry{
			data:      []byte("payload"),
			expiresAt: time.Now().Add(time.Minute),
		}

		err := store.Delete(t.Context(), "")

		if err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
		if _, ok := store.entries[""]; !ok {
			t.Error("want store.entries[\"\"] ok: true; got: false")
		}
	})

	t.Run("rejects the call when the context is already done", func(t *testing.T) {
		store := New()
		if _, err := store.Create(t.Context(), "id-1", []byte("payload"), time.Minute); err != nil {
			t.Fatalf("store.Create: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Delete(ctx, "id-1")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
		data, err := store.Get(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("store.Get: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the ID is empty and the context is already done", func(t *testing.T) {
		store := New()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Delete(ctx, "")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
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
		wg.Add(groups * 4)

		for g := range groups {
			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]
					data := []byte(id + ":" + strconv.Itoa(g))

					_, err := store.Create(t.Context(), id, data, time.Minute)

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
					data := []byte(id + ":" + strconv.Itoa(g))

					_, err := store.Update(t.Context(), id, data, time.Minute)

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

					data, err := store.Get(t.Context(), id)

					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
					if data == nil {
						continue
					}
					if want, got := id+":", string(data); !strings.HasPrefix(got, want) {
						t.Errorf("want data: starting with %q; got: %q", want, got)
						return
					}
				}
			}()

			go func() {
				defer wg.Done()

				for i := range iterations {
					id := ids[(g+i)%len(ids)]

					err := store.Delete(t.Context(), id)

					if err != nil {
						t.Errorf("want err: nil; got: %v", err)
						return
					}
				}
			}()
		}

		wg.Wait()
	})
}
