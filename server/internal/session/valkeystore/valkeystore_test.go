package valkeystore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideconfig "github.com/valkey-io/valkey-glide/go/v2/config"
	glideopts "github.com/valkey-io/valkey-glide/go/v2/options"

	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// Asserts at compile time that *Store satisfies session.Store.
var _ session.Store = (*Store)(nil)

// valkeyURL addresses the container shared by every test in this package. Tests
// isolate by key prefix, since starting a container per test costs seconds.
var valkeyURL *url.URL

// TestMain runs a single Valkey for the whole package. 8.1 is the newest
// version offered by both ElastiCache and upstream, so local and deployed can
// be pinned alike. AWS also offers an 8.2, which upstream never published.
func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := testcontainers.Run(ctx, "valkey/valkey:8.1-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers.Run: %v\n", err)
		os.Exit(1)
	}

	endpoint, err := container.PortEndpoint(ctx, "6379/tcp", "")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintf(os.Stderr, "container.PortEndpoint: %v\n", err)
		os.Exit(1)
	}

	valkeyURL, err = url.Parse("valkey://" + endpoint)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintf(os.Stderr, "url.Parse: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = testcontainers.TerminateContainer(container)
	os.Exit(code)
}

func newClient(t *testing.T) *glide.Client {
	t.Helper()

	port, err := strconv.Atoi(valkeyURL.Port())
	if err != nil {
		t.Fatalf("strconv.Atoi: %v", err)
	}

	client, err := glide.NewClient(glideconfig.NewClientConfiguration().
		WithAddress(&glideconfig.NodeAddress{Host: valkeyURL.Hostname(), Port: port}))
	if err != nil {
		t.Fatalf("glide.NewClient: %v", err)
	}
	t.Cleanup(client.Close)

	return client
}

// newStore namespaces the store to the calling test, so tests cannot collide.
func newStore(t *testing.T, client *glide.Client) *Store {
	t.Helper()

	return New(client, WithPrefix(t.Name()+":"))
}

// seed writes through the client, so setup never runs through a method under test.
func seed(t *testing.T, client *glide.Client, key, value string, ttl time.Duration) {
	t.Helper()

	if _, err := client.SetWithOptions(t.Context(), key, value, glideopts.SetOptions{
		Expiry: glideopts.NewExpiryIn(ttl),
	}); err != nil {
		t.Fatalf("client.SetWithOptions: %v", err)
	}
}

// stored reads through the client, so assertions never run through a method under test.
func stored(t *testing.T, client *glide.Client, key string) (string, bool) {
	t.Helper()

	result, err := client.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	if result.IsNil() {
		return "", false
	}

	return result.Value(), true
}

func sessionCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}

	return nil
}

func TestNew(t *testing.T) {
	t.Run("applies the default prefix", func(t *testing.T) {
		store := New(nil)

		if want, got := "session:", store.prefix; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("applies options", func(t *testing.T) {
		store := New(nil, WithPrefix("custom:"))

		if want, got := "custom:", store.prefix; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}

func TestStore_Prepare(t *testing.T) {
	client := newClient(t)

	t.Run("returns (nil, nil) for unknown id", func(t *testing.T) {
		store := newStore(t, client)

		result, err := store.Prepare(t.Context(), "missing")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns (nil, nil) for empty id", func(t *testing.T) {
		store := newStore(t, client)

		result, err := store.Prepare(t.Context(), "")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns the stored snapshot", func(t *testing.T) {
		store := newStore(t, client)
		seed(t, client, store.prefix+"id-1",
			`{"id":"id-1","csrf_token":"csrf-1","user":{"email":"alice@example.com"},"data":{"k":"v"}}`,
			time.Minute)

		result, err := store.Prepare(t.Context(), "id-1")

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

	t.Run("treats an undecodable entry as absent", func(t *testing.T) {
		// The entry survives the process that wrote it, so an error here would
		// fail every request carrying this ID until the TTL elapses, and the
		// middleware never issues a replacement cookie to escape it.
		store := newStore(t, client)
		seed(t, client, store.prefix+"id-1", "not json", time.Minute)

		result, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("does not read across prefixes", func(t *testing.T) {
		// One Valkey can back more than one consumer, so the prefix is the only
		// thing keeping their keyspaces apart.
		store := New(client, WithPrefix(t.Name()+":b:"))
		seed(t, client, t.Name()+":a:id-1", `{"id":"id-1","csrf_token":"csrf-1"}`, time.Minute)

		result, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})
}

func TestStore_TTL(t *testing.T) {
	client := newClient(t)

	t.Run("expires entry after TTL elapses", func(t *testing.T) {
		// Valkey expires keys server side, so there is no clock to inject the
		// way memstore allows. A short real TTL is the only way to observe it.
		store := newStore(t, client)
		seed(t, client, store.prefix+"id-1", `{"id":"id-1","csrf_token":"csrf-1"}`, 50*time.Millisecond)

		time.Sleep(150 * time.Millisecond)

		result, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result != nil {
			t.Errorf("want: nil; got: %+v", result)
		}
	})

	t.Run("returns the entry while still within TTL", func(t *testing.T) {
		store := newStore(t, client)
		seed(t, client, store.prefix+"id-1", `{"id":"id-1","csrf_token":"csrf-1"}`, time.Minute)

		result, err := store.Prepare(t.Context(), "id-1")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result == nil {
			t.Fatal("want: non-nil; got: nil")
		}
	})
}

func TestStore_Commit(t *testing.T) {
	client := newClient(t)

	t.Run("stores snapshot under its prefixed ID with an expiry", func(t *testing.T) {
		store := newStore(t, client)
		snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-1"}

		if err := store.Commit(t.Context(), snapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		data, ok := stored(t, client, store.prefix+"id-1")
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want, got := `{"id":"id-1","csrf_token":"csrf-1"}`, data; want != got {
			t.Errorf("want: %s; got: %s", want, got)
		}

		// A missing expiry would leave sessions in Valkey forever.
		ttl, err := client.TTL(t.Context(), store.prefix+"id-1")
		if err != nil {
			t.Fatalf("client.TTL: %v", err)
		}
		if got := ttl; got <= 0 {
			t.Errorf("want: positive; got: %d", got)
		}
	})

	t.Run("overwrites an existing entry", func(t *testing.T) {
		store := newStore(t, client)
		seed(t, client, store.prefix+"id-1", `{"id":"id-1","csrf_token":"csrf-1"}`, time.Minute)

		snapshot := &session.Snapshot{ID: "id-1", CSRFToken: "csrf-2"}
		if err := store.Commit(t.Context(), snapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		data, ok := stored(t, client, store.prefix+"id-1")
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want, got := `{"id":"id-1","csrf_token":"csrf-2"}`, data; want != got {
			t.Errorf("want: %s; got: %s", want, got)
		}
	})

	t.Run("rejects nil snapshot", func(t *testing.T) {
		store := newStore(t, client)

		err := store.Commit(t.Context(), nil, time.Minute)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "snap must be non-nil"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("rejects empty snap.ID", func(t *testing.T) {
		store := newStore(t, client)

		err := store.Commit(t.Context(), &session.Snapshot{}, time.Minute)

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
				store := newStore(t, client)

				err := store.Commit(t.Context(), &session.Snapshot{ID: "id-1"}, tt.ttl)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if want := "ttl must be positive"; !strings.Contains(err.Error(), want) {
					t.Errorf("want err: containing %q; got: %q", want, err)
				}
				if _, ok := stored(t, client, store.prefix+"id-1"); ok {
					t.Error("want ok: false; got: true")
				}
			})
		}
	})
}

func TestStore_RoundTrip(t *testing.T) {
	client := newClient(t)

	t.Run("returns data in its JSON form", func(t *testing.T) {
		// Values come back shaped by the encoding, not as the Go types that
		// went in, so callers must assert against the decoded form.
		store := newStore(t, client)
		snapshot := &session.Snapshot{
			ID:        "id-1",
			CSRFToken: "csrf-1",
			Data:      map[string]any{"count": 1},
		}

		if err := store.Commit(t.Context(), snapshot, time.Minute); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		result, err := store.Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if result == nil {
			t.Fatal("want: non-nil; got: nil")
		}

		if want, got := float64(1), result.Data["count"]; want != got {
			t.Errorf("want: %[1]v (%[1]T); got: %[2]v (%[2]T)", want, got)
		}
	})
}

func TestStore_Drop(t *testing.T) {
	client := newClient(t)

	t.Run("removes a stored entry", func(t *testing.T) {
		store := newStore(t, client)
		seed(t, client, store.prefix+"id-1", `{"id":"id-1","csrf_token":"csrf-1"}`, time.Minute)

		if err := store.Drop(t.Context(), "id-1"); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		if _, ok := stored(t, client, store.prefix+"id-1"); ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("is a no-op on an unknown id", func(t *testing.T) {
		store := newStore(t, client)

		if err := store.Drop(t.Context(), "missing"); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("is a no-op on empty id", func(t *testing.T) {
		store := newStore(t, client)

		if err := store.Drop(t.Context(), ""); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})
}

func TestStore_SharedAcrossInstances(t *testing.T) {
	t.Run("carries session data between two server instances", func(t *testing.T) {
		// Two middleware chains over two independent clients to one Valkey stand
		// in for two server processes behind a load balancer. They share no
		// in-process state, so a value reaching the second can only have
		// travelled through the store.
		const (
			cookieName = "tw_session"
			key        = "school"
			value      = "Greendale Secondary"
		)

		prefix := t.Name() + ":"
		opts := middleware.SessionOptions{
			Name:             cookieName,
			DefaultTTL:       time.Minute,
			AuthenticatedTTL: time.Minute,
		}
		instanceA := middleware.Session(New(newClient(t), WithPrefix(prefix)), opts)
		instanceB := middleware.Session(New(newClient(t), WithPrefix(prefix)), opts)

		writer := instanceA(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := middleware.SessionFromContext(r.Context())
			if !ok {
				t.Error("want ok: true; got: false")
				return
			}
			// A string sidesteps the JSON round trip turning numbers into float64.
			sess.Set(key, value)
			w.WriteHeader(http.StatusOK)
		}))

		recA := httptest.NewRecorder()
		writer.ServeHTTP(recA, httptest.NewRequest(http.MethodGet, "/", nil))

		issued := sessionCookie(recA, cookieName)
		if issued == nil {
			t.Fatal("want cookie: non-nil; got: nil")
		}

		var (
			got   any
			found bool
		)
		reader := instanceB(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := middleware.SessionFromContext(r.Context())
			if !ok {
				t.Error("want ok: true; got: false")
				return
			}
			got, found = sess.Get(key)
			w.WriteHeader(http.StatusOK)
		}))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.AddCookie(issued)
		recB := httptest.NewRecorder()
		reader.ServeHTTP(recB, request)

		if !found {
			t.Fatal("want ok: true; got: false")
		}
		if want := value; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}

		// Continuing the same session, rather than issuing a replacement, is
		// what separates a store read from a fresh start.
		returned := sessionCookie(recB, cookieName)
		if returned == nil {
			t.Fatal("want cookie: non-nil; got: nil")
		}
		if want, got := issued.Value, returned.Value; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}
