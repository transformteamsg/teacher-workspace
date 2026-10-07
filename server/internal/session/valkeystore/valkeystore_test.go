package valkeystore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideconfig "github.com/valkey-io/valkey-glide/go/v2/config"
	glideopts "github.com/valkey-io/valkey-glide/go/v2/options"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// Asserts at compile time that *Store satisfies session.Store.
var _ session.Store = (*Store)(nil)

func TestStore_Prepare(t *testing.T) {
	t.Parallel()

	t.Run("returns (nil, nil) when the ID has no entry", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "payload",
		})
		store := New(client)

		data, err := store.Prepare(t.Context(), "missing")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("returns (nil, nil) when the ID is empty", func(t *testing.T) {
		t.Parallel()

		// Seeded at the bare prefix, so it is read only if an empty id reaches Valkey.
		client := newValkeyClient(t, entry{
			key:   "session:",
			value: "payload",
		})
		store := New(client)

		data, err := store.Prepare(t.Context(), "")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if data != nil {
			t.Error("want data: nil; got: non-nil")
		}
	})

	t.Run("returns the bytes last committed under the ID", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "payload",
		})
		store := New(client)

		data, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("reads only under its own prefix", func(t *testing.T) {
		t.Parallel()

		// Seeded under both prefixes with different values, so the data shows which key was read.
		client := newValkeyClient(t,
			entry{
				key:   "session:id-1",
				value: "default",
			},
			entry{
				key:   "test:id-1",
				value: "payload",
			},
		)
		store := New(client, WithPrefix("test:"))

		data, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "payload", string(data); want != got {
			t.Errorf("want data: %q; got: %q", want, got)
		}
	})

	t.Run("returns an error when the client is closed", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)
		client.Close()

		_, err := store.Prepare(t.Context(), "id-1")

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "prepare session"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("returns the context's error when the context is already done", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Prepare(ctx, "id-1")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
	})

	t.Run("returns the context's error when the ID is empty and the context is already done", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := store.Prepare(ctx, "")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}
	})
}

func TestStore_Commit(t *testing.T) {
	t.Parallel()

	t.Run("stores data under the ID for the TTL", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)

		err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "payload", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}

		ttl, err := client.PTTL(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.PTTL: %v", err)
		}
		if want, got := time.Minute, time.Duration(ttl)*time.Millisecond; want < got {
			t.Errorf("want TTL: <= %v; got: %v", want, got)
		}
		if want, got := 50*time.Second, time.Duration(ttl)*time.Millisecond; want >= got {
			t.Errorf("want TTL: > %v; got: %v", want, got)
		}
	})

	t.Run("replaces an existing entry", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "old",
			ttl:   10 * time.Second,
		})
		store := New(client)

		err := store.Commit(t.Context(), "id-1", []byte("new"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "new", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}

		ttl, err := client.PTTL(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.PTTL: %v", err)
		}
		if want, got := 10*time.Second, time.Duration(ttl)*time.Millisecond; want >= got {
			t.Errorf("want TTL: > %v; got: %v", want, got)
		}
	})

	t.Run("stores data containing NUL and invalid UTF-8 bytes unchanged", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)
		// Values are opaque bytes, so a NUL byte and invalid UTF-8 (0xff, 0xfe)
		// must reach Valkey unchanged.
		data := []byte{0x00, 0xff, 'a', 0xfe}

		err := store.Commit(t.Context(), "id-1", data, time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := string(data), stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}
	})

	t.Run("stores data under its own prefix", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client, WithPrefix("test:"))

		err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "test:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "payload", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}
	})

	t.Run("rejects an empty ID", func(t *testing.T) {
		t.Parallel()

		// Seeded at the bare prefix, so it changes only if an empty id reaches Valkey.
		client := newValkeyClient(t, entry{
			key:   "session:",
			value: "old",
		})
		store := New(client)

		err := store.Commit(t.Context(), "", []byte("payload"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "id must be non-empty"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}

		stored, err := client.Get(t.Context(), "session:")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "old", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}
	})

	t.Run("rejects a TTL that is", func(t *testing.T) {
		t.Parallel()

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
				t.Parallel()

				client := newValkeyClient(t, entry{
					key:   "session:id-1",
					value: "old",
				})
				store := New(client)

				err := store.Commit(t.Context(), "id-1", []byte("payload"), test.ttl)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if want := "ttl must be positive"; !strings.Contains(err.Error(), want) {
					t.Errorf("want err: containing %q; got: %q", want, err)
				}

				stored, err := client.Get(t.Context(), "session:id-1")
				if err != nil {
					t.Fatalf("client.Get: %v", err)
				}
				if want, got := "old", stored.Value(); want != got {
					t.Errorf("want stored.Value(): %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("returns an error when the client is closed", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)
		client.Close()

		err := store.Commit(t.Context(), "id-1", []byte("payload"), time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "commit session"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("rejects the call when the context is already done", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "old",
		})
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Commit(ctx, "id-1", []byte("new"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "old", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the ID is empty and the context is already done", func(t *testing.T) {
		t.Parallel()

		// Seeded at the bare prefix, so it changes only if an empty id reaches Valkey.
		client := newValkeyClient(t, entry{
			key:   "session:",
			value: "old",
		})
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Commit(ctx, "", []byte("new"), time.Minute)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}

		stored, err := client.Get(t.Context(), "session:")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "old", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}
	})

	t.Run("rejects the call when the TTL is not positive and the context is already done", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "old",
		})
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Commit(ctx, "id-1", []byte("new"), 0)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if want, got := "old", stored.Value(); want != got {
			t.Errorf("want stored.Value(): %q; got: %q", want, got)
		}
	})
}

func TestStore_Drop(t *testing.T) {
	t.Parallel()

	t.Run("removes the entry under the ID", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "payload",
		})
		store := New(client)

		err := store.Drop(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if !stored.IsNil() {
			t.Error("want stored.IsNil(): true; got: false")
		}
	})

	t.Run("is a no-op when the ID has no entry", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "payload",
		})
		store := New(client)

		err := store.Drop(t.Context(), "missing")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if stored.IsNil() {
			t.Error("want stored.IsNil(): false; got: true")
		}
	})

	t.Run("is a no-op when the ID is empty", func(t *testing.T) {
		t.Parallel()

		// Seeded at the bare prefix, so it is deleted only if an empty id reaches Valkey.
		client := newValkeyClient(t, entry{
			key:   "session:",
			value: "payload",
		})
		store := New(client)

		err := store.Drop(t.Context(), "")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		stored, err := client.Get(t.Context(), "session:")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if stored.IsNil() {
			t.Error("want stored.IsNil(): false; got: true")
		}
	})

	t.Run("removes only under its own prefix", func(t *testing.T) {
		t.Parallel()

		// Seeded under both prefixes, so the entry left behind shows which key was deleted.
		client := newValkeyClient(t,
			entry{
				key:   "session:id-1",
				value: "payload",
			},
			entry{
				key:   "test:id-1",
				value: "payload",
			},
		)
		store := New(client, WithPrefix("test:"))

		err := store.Drop(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		dropped, err := client.Get(t.Context(), "test:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if !dropped.IsNil() {
			t.Error("want dropped.IsNil(): true; got: false")
		}

		kept, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if kept.IsNil() {
			t.Error("want kept.IsNil(): false; got: true")
		}
	})

	t.Run("returns an error when the client is closed", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t)
		store := New(client)
		client.Close()

		err := store.Drop(t.Context(), "id-1")

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "drop session"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("rejects the call when the context is already done", func(t *testing.T) {
		t.Parallel()

		client := newValkeyClient(t, entry{
			key:   "session:id-1",
			value: "payload",
		})
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Drop(ctx, "id-1")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}

		stored, err := client.Get(t.Context(), "session:id-1")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if stored.IsNil() {
			t.Error("want stored.IsNil(): false; got: true")
		}
	})

	t.Run("rejects the call when the ID is empty and the context is already done", func(t *testing.T) {
		t.Parallel()

		// Seeded at the bare prefix, so it is deleted only if an empty id reaches Valkey.
		client := newValkeyClient(t, entry{
			key:   "session:",
			value: "payload",
		})
		store := New(client)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := store.Drop(ctx, "")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("want err: %v; got: %v", context.Canceled, err)
		}

		stored, err := client.Get(t.Context(), "session:")
		if err != nil {
			t.Fatalf("client.Get: %v", err)
		}
		if stored.IsNil() {
			t.Error("want stored.IsNil(): false; got: true")
		}
	})
}

// entry is a key written to Valkey before a test runs.
type entry struct {
	key   string
	value string
	// ttl is the key's lifetime. Zero leaves the key without an expiry.
	ttl time.Duration
}

// newValkeyClient returns a client connected to a Valkey server dedicated to
// the calling test, so every test begins with a keyspace holding only seeds.
// The client and server are released when the test ends.
func newValkeyClient(t *testing.T, seeds ...entry) *glide.Client {
	t.Helper()

	container, err := testcontainers.Run(t.Context(), "valkey/valkey:9.1-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("testcontainers.Run: %v", err)
	}

	host, err := container.Host(t.Context())
	if err != nil {
		t.Fatalf("container.Host: %v", err)
	}
	port, err := container.MappedPort(t.Context(), "6379/tcp")
	if err != nil {
		t.Fatalf("container.MappedPort: %v", err)
	}

	client, err := glide.NewClient(glideconfig.NewClientConfiguration().
		WithAddress(&glideconfig.NodeAddress{
			Host: host,
			Port: int(port.Num()),
		}))
	if err != nil {
		t.Fatalf("glide.NewClient: %v", err)
	}
	t.Cleanup(client.Close)

	for _, e := range seeds {
		var opts glideopts.SetOptions
		if e.ttl != 0 {
			opts.Expiry = glideopts.NewExpiryIn(e.ttl)
		}
		if _, err := client.SetWithOptions(t.Context(), e.key, e.value, opts); err != nil {
			t.Fatalf("client.SetWithOptions: %v", err)
		}
	}

	return client
}
