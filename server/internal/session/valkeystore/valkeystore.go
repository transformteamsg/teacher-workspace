// Package valkeystore implements session.Store backed by Valkey. It is
// intended for deployments where session state must outlive a single process
// and be shared across instances.
package valkeystore

import (
	"context"
	"fmt"
	"time"

	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideopts "github.com/valkey-io/valkey-glide/go/v2/options"
)

const defaultPrefix = "session:"

// Option configures a Store.
type Option func(*Store)

// WithPrefix sets the storage-key prefix. Default is "session:".
func WithPrefix(prefix string) Option {
	return func(s *Store) { s.prefix = prefix }
}

// Store is a Valkey-backed session.Store.
type Store struct {
	client *glide.Client
	prefix string
}

// New returns a Valkey-backed Store.
func New(client *glide.Client, opts ...Option) *Store {
	s := &Store{
		client: client,
		prefix: defaultPrefix,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Prepare implements [session.Store.Prepare].
func (s *Store) Prepare(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("valkeystore: prepare session: %w", err)
	}
	if id == "" {
		return nil, nil
	}

	result, err := s.client.Get(ctx, s.prefix+id)
	if err != nil {
		return nil, fmt.Errorf("valkeystore: prepare session: %w", err)
	}
	// A key that has passed its TTL reads as absent.
	if result.IsNil() {
		return nil, nil
	}

	return []byte(result.Value()), nil
}

// Commit implements [session.Store.Commit].
func (s *Store) Commit(ctx context.Context, id string, data []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("valkeystore: commit session: %w", err)
	}
	if id == "" {
		return fmt.Errorf("valkeystore: id must be non-empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("valkeystore: ttl must be positive, got %v", ttl)
	}

	if _, err := s.client.SetWithOptions(ctx, s.prefix+id, string(data), glideopts.SetOptions{
		Expiry: glideopts.NewExpiryIn(ttl),
	}); err != nil {
		return fmt.Errorf("valkeystore: commit session: %w", err)
	}

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("valkeystore: drop session: %w", err)
	}
	if id == "" {
		return nil
	}

	if _, err := s.client.Del(ctx, []string{s.prefix + id}); err != nil {
		return fmt.Errorf("valkeystore: drop session: %w", err)
	}

	return nil
}
