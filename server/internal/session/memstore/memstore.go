// Package memstore implements session.Store in memory. Entries are copied in
// and out, so callers share no state with the store or each other. It is
// intended for development and tests; production deployments should use a
// shared store.
package memstore

import (
	"bytes"
	"context"
	"errors"
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

// Store is an in-memory session.Store. An expired entry is freed the next time
// a method is called with its ID, so the entry of an ID that is never used
// again stays in memory.
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

// Get implements [session.Store.Get].
func (s *Store) Get(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("memstore: get: %w", err)
	}
	if id == "" {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.lookup(id)
	if !ok {
		return nil, nil
	}

	return bytes.Clone(e.data), nil
}

// Create implements [session.Store.Create].
func (s *Store) Create(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("memstore: create: %w", err)
	}
	if id == "" {
		return false, errors.New("memstore: create: id must be non-empty")
	}
	if len(data) == 0 {
		return false, errors.New("memstore: create: data must be non-empty")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("memstore: create: ttl must be positive; got %v", ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.lookup(id); ok {
		return false, nil
	}

	s.entries[id] = entry{
		data:      bytes.Clone(data),
		expiresAt: s.now().Add(ttl),
	}

	return true, nil
}

// Update implements [session.Store.Update].
func (s *Store) Update(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("memstore: update: %w", err)
	}
	if id == "" {
		return false, errors.New("memstore: update: id must be non-empty")
	}
	if len(data) == 0 {
		return false, errors.New("memstore: update: data must be non-empty")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("memstore: update: ttl must be positive; got %v", ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.lookup(id); !ok {
		return false, nil
	}

	s.entries[id] = entry{
		data:      bytes.Clone(data),
		expiresAt: s.now().Add(ttl),
	}

	return true, nil
}

// Delete implements [session.Store.Delete].
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("memstore: delete: %w", err)
	}
	if id == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, id)

	return nil
}

// lookup returns the entry under id, and reports whether there is one that has
// not expired. It frees an expired entry. The caller must hold s.mu.
func (s *Store) lookup(id string) (entry, bool) {
	e, ok := s.entries[id]
	if !ok {
		return entry{}, false
	}

	if !e.expiresAt.After(s.now()) {
		delete(s.entries, id)
		return entry{}, false
	}

	return e, true
}
