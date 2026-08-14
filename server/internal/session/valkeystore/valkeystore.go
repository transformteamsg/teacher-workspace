// Package valkeystore implements session.Store backed by Valkey. Snapshots are
// JSON-encoded under a caller-configurable key prefix and expire server-side
// when their TTL elapses, so no sweeper is needed. It is intended for
// deployments where session state must outlive a single process and be shared
// across instances.
package valkeystore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideopts "github.com/valkey-io/valkey-glide/go/v2/options"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// Option configures a Store.
type Option func(*Store)

// WithPrefix sets the storage-key prefix. Default is "session:".
func WithPrefix(prefix string) Option {
	return func(s *Store) { s.prefix = prefix }
}

// Store is a Valkey-backed session.Store. It does not own the client it is
// given, so closing the client is the caller's responsibility.
type Store struct {
	client *glide.Client
	prefix string
}

// New returns a Store that uses client to talk to Valkey. The default key
// prefix is "session:"; override with WithPrefix.
func New(client *glide.Client, opts ...Option) *Store {
	s := &Store{client: client, prefix: "session:"}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Prepare implements [session.Store.Prepare]. An entry that cannot be decoded
// is logged and reported as absent, so a session written by an incompatible
// build starts over rather than failing every request until its TTL elapses.
func (s *Store) Prepare(ctx context.Context, id string) (*session.Snapshot, error) {
	if id == "" {
		return nil, nil
	}

	result, err := s.client.Get(ctx, s.prefix+id)
	if err != nil {
		return nil, fmt.Errorf("valkeystore: prepare session: %w", err)
	}
	// A key that has passed its TTL reads as absent, so expiry needs no special
	// handling here.
	if result.IsNil() {
		return nil, nil
	}

	var snap session.Snapshot
	if err := json.Unmarshal([]byte(result.Value()), &snap); err != nil {
		// An entry that cannot be decoded outlives the process that wrote it, so
		// surfacing an error would fail every request carrying this ID until the
		// TTL elapses, with no cookie issued to escape it. Starting a fresh
		// session is what expiry already does with an unusable entry.
		slog.ErrorContext(ctx, "valkeystore: discarding undecodable session", "err", err)
		return nil, nil
	}

	return &snap, nil
}

// Commit implements [session.Store.Commit].
func (s *Store) Commit(ctx context.Context, snap *session.Snapshot, ttl time.Duration) error {
	if snap == nil {
		return fmt.Errorf("valkeystore: snap must be non-nil")
	}
	if snap.ID == "" {
		return fmt.Errorf("valkeystore: snap.ID must be non-empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("valkeystore: ttl must be positive, got %v", ttl)
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("valkeystore: marshal snapshot: %w", err)
	}

	if _, err := s.client.SetWithOptions(ctx, s.prefix+snap.ID, string(data), glideopts.SetOptions{
		Expiry: glideopts.NewExpiryIn(ttl),
	}); err != nil {
		return fmt.Errorf("valkeystore: commit session: %w", err)
	}

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}

	if _, err := s.client.Del(ctx, []string{s.prefix + id}); err != nil {
		return fmt.Errorf("valkeystore: drop session: %w", err)
	}

	return nil
}
