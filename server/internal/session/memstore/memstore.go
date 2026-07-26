// Package memstore implements session.Store with an in-process map. Entries
// are evicted lazily on read once their TTL elapses. It is intended for
// development and tests; production deployments should use a shared store.
package memstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/session"
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
	snap      *session.Snapshot
	expiresAt time.Time
}

// New returns a Store with an optional clock override.
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
func (s *Store) Prepare(_ context.Context, id string) (*session.Snapshot, error) {
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

	return e.snap, nil
}

// Commit implements [session.Store.Commit].
func (s *Store) Commit(_ context.Context, snap *session.Snapshot, ttl time.Duration) error {
	if snap == nil {
		return fmt.Errorf("memstore: snap must be non-nil")
	}
	if snap.ID == "" {
		return fmt.Errorf("memstore: snap.ID must be non-empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("memstore: ttl must be positive, got %v", ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[snap.ID] = entry{snap: snap, expiresAt: s.now().Add(ttl)}

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, id)

	return nil
}
