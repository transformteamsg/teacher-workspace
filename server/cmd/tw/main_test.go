package main

import (
	"net/url"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/String-sg/teacher-workspace/server/internal/session/memstore"
	"github.com/String-sg/teacher-workspace/server/internal/session/valkeystore"
)

func TestNewSessionStore(t *testing.T) {
	t.Run("returns an in-memory store for the memory provider", func(t *testing.T) {
		cfg := config.Default()

		store, cleanup, err := newSessionStore(&cfg)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.(*memstore.Store); !ok {
			t.Errorf("want: *memstore.Store; got: %T", store)
		}
		if cleanup == nil {
			t.Error("want cleanup: non-nil; got: nil")
		}
	})

	t.Run("returns a Valkey store for the valkey provider", func(t *testing.T) {
		container, err := testcontainers.Run(t.Context(), "valkey/valkey:8.1-alpine",
			testcontainers.WithExposedPorts("6379/tcp"),
			testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")),
		)
		if err != nil {
			t.Fatalf("testcontainers.Run: %v", err)
		}
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

		endpoint, err := container.PortEndpoint(t.Context(), "6379/tcp", "")
		if err != nil {
			t.Fatalf("container.PortEndpoint: %v", err)
		}
		address, err := url.Parse("valkey://" + endpoint)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.Session.StoreProvider = config.StoreProviderValkey
		cfg.Session.ValkeyURL = address
		cfg.Session.ValkeyPrefix = "configured:"

		store, cleanup, err := newSessionStore(&cfg)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if _, ok := store.(*valkeystore.Store); !ok {
			t.Errorf("want: *valkeystore.Store; got: %T", store)
		}
		if cleanup == nil {
			t.Fatal("want cleanup: non-nil; got: nil")
		}
		t.Cleanup(cleanup)

		reader, err := valkeystore.Dial(t.Context(), address)
		if err != nil {
			t.Fatalf("valkeystore.Dial: %v", err)
		}
		t.Cleanup(reader.Close)

		if err := store.Commit(t.Context(), &session.Snapshot{ID: "id-1"}, time.Minute); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		snap, err := valkeystore.New(reader, valkeystore.WithPrefix("configured:")).Prepare(t.Context(), "id-1")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		if snap == nil {
			t.Error("want: entry under the configured prefix; got: nil")
		}
	})

	t.Run("honours the configured dial timeout", func(t *testing.T) {
		// glide.NewClient takes no context, so the configured timeout applies
		// only because Dial hands it over as a connection timeout. Without that
		// the attempt falls back to glide's own default of roughly 2s, which
		// the bound below is set to reject.
		cfg := config.Default()
		cfg.Session.StoreProvider = config.StoreProviderValkey
		cfg.Session.ValkeyURL = &url.URL{Scheme: "valkey", Host: "127.0.0.1:1"}
		cfg.Session.ValkeyDialTimeout = 300 * time.Millisecond

		start := time.Now()
		_, _, err := newSessionStore(&cfg)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := 1500 * time.Millisecond; elapsed > want {
			t.Errorf("want: under %v; got: %v", want, elapsed)
		}
	})

	t.Run("returns an error when Valkey is unreachable", func(t *testing.T) {
		cfg := config.Default()
		cfg.Session.StoreProvider = config.StoreProviderValkey
		// Port 1 is reserved and never has a listener.
		cfg.Session.ValkeyURL = &url.URL{Scheme: "valkey", Host: "127.0.0.1:1"}
		cfg.Session.ValkeyDialTimeout = 300 * time.Millisecond

		store, cleanup, err := newSessionStore(&cfg)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if store != nil {
			t.Errorf("want: nil; got: %T", store)
		}
		if cleanup != nil {
			t.Error("want cleanup: nil; got: non-nil")
		}
	})
}
