// Package valkeystore implements session.Store backed by Valkey. It is
// intended for deployments where session state must outlive a single process
// and be shared across instances.
package valkeystore

import (
	"context"
	"errors"
	"fmt"
	"time"

	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideconstants "github.com/valkey-io/valkey-glide/go/v2/constants"
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

// Get implements [session.Store.Get].
func (s *Store) Get(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("valkeystore: get: %w", err)
	}
	if id == "" {
		return nil, nil
	}

	result, err := s.client.Get(ctx, s.prefix+id)
	if err != nil {
		return nil, fmt.Errorf("valkeystore: get: %w", err)
	}
	// A key that has passed its TTL reads as absent.
	if result.IsNil() {
		return nil, nil
	}

	return []byte(result.Value()), nil
}

// Create implements [session.Store.Create].
func (s *Store) Create(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("valkeystore: create: %w", err)
	}
	if id == "" {
		return false, errors.New("valkeystore: create: id must be non-empty")
	}
	if len(data) == 0 {
		return false, errors.New("valkeystore: create: data must be non-empty")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("valkeystore: create: ttl must be positive; got %v", ttl)
	}

	result, err := s.client.SetWithOptions(ctx, s.prefix+id, string(data), glideopts.SetOptions{
		ConditionalSet: glideconstants.OnlyIfDoesNotExist,
		Expiry:         glideopts.NewExpiryIn(ttl),
	})
	if err != nil {
		return false, fmt.Errorf("valkeystore: create: %w", err)
	}

	// SET replies nil when its condition stops the write.
	return !result.IsNil(), nil
}

// Update implements [session.Store.Update].
func (s *Store) Update(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("valkeystore: update: %w", err)
	}
	if id == "" {
		return false, errors.New("valkeystore: update: id must be non-empty")
	}
	if len(data) == 0 {
		return false, errors.New("valkeystore: update: data must be non-empty")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("valkeystore: update: ttl must be positive; got %v", ttl)
	}

	result, err := s.client.SetWithOptions(ctx, s.prefix+id, string(data), glideopts.SetOptions{
		ConditionalSet: glideconstants.OnlyIfExists,
		Expiry:         glideopts.NewExpiryIn(ttl),
	})
	if err != nil {
		return false, fmt.Errorf("valkeystore: update: %w", err)
	}

	// SET replies nil when its condition stops the write.
	return !result.IsNil(), nil
}

// Delete implements [session.Store.Delete].
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("valkeystore: delete: %w", err)
	}
	if id == "" {
		return nil
	}

	if _, err := s.client.Del(ctx, []string{s.prefix + id}); err != nil {
		return fmt.Errorf("valkeystore: delete: %w", err)
	}

	return nil
}
