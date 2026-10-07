// Package memstore implements session.Store in memory. Entries are copied in
// and out, so callers share no state with the store or each other. It is
// intended for development and tests; production deployments should use a
// shared store.
package memstore

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"
)

// Option configures a Store.
type Option func(*Store)

// WithClock injects the time source used to evaluate TTLs. Tests use this to
// advance time without sleeping.
func WithClock(now func() time.Time) Option {
	return func(s *Store) {
		s.now = now
	}
}

// Store is an in-memory session.Store.
type Store struct {
	mu      sync.Mutex
	entries map[string]entry
	now     func() time.Time
}

type entry struct {
	data      []byte
	expiresAt time.Time
}

// New returns an in-memory Store.
func New(opts ...Option) *Store {
	s := &Store{
		entries: make(map[string]entry),
		now:     time.Now,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Prepare implements [session.Store.Prepare].
func (s *Store) Prepare(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("memstore: prepare session: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return nil, nil
	}

	if !e.expiresAt.After(s.now()) {
		delete(s.entries, id)
		return nil, nil
	}

	return bytes.Clone(e.data), nil
}

// Commit implements [session.Store.Commit].
func (s *Store) Commit(ctx context.Context, id string, data []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("memstore: commit session: %w", err)
	}
	if id == "" {
		return fmt.Errorf("memstore: id must be non-empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("memstore: ttl must be positive, got %v", ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[id] = entry{data: bytes.Clone(data), expiresAt: s.now().Add(ttl)}

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("memstore: drop session: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, id)

	return nil
}
