// Package memstore implements session.Store with an in-process map. Snapshots
// are stored as JSON, so callers share no state with the store or each other.
// Entries are evicted lazily on read once their TTL elapses, and at the store's
// entry or byte limit the oldest unauthenticated ones go first. It is intended
// for development and tests; production deployments should use a shared store.
package memstore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

const (
	// defaultMaxEntries and defaultMaxBytes bound a store built without
	// limits. A session holds ~280 bytes signed out and ~1.5 KB with a sign-in
	// underway, so the byte limit is what binds first in the worst case.
	defaultMaxEntries = 50_000
	defaultMaxBytes   = 64 << 20

	// cullBatch and cullDivisor bound how much a cull frees beyond the write
	// that triggered it: at most 64 entries, and at most a tenth of the entry
	// limit. Freeing a batch amortises the scan it costs over the writes that
	// follow, and capping the batch keeps a store whose byte limit binds first
	// from signing out thousands of visitors to make room for one.
	cullBatch   = 64
	cullDivisor = 10
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

// WithMaxEntries caps how many sessions the store holds. Values below 1 leave
// the default in place.
func WithMaxEntries(maxEntries int) Option {
	return func(s *Store) {
		if maxEntries > 0 {
			s.maxEntries = maxEntries
		}
	}
}

// WithMaxBytes caps the total size of the snapshots the store holds. Values
// below 1 leave the default in place.
func WithMaxBytes(maxBytes int) Option {
	return func(s *Store) {
		if maxBytes > 0 {
			s.maxBytes = maxBytes
		}
	}
}

// Store is an in-memory session.Store.
type Store struct {
	mu         sync.Mutex
	entries    map[string]entry
	bytes      int
	maxEntries int
	maxBytes   int
	now        func() time.Time
}

type entry struct {
	data      []byte
	expiresAt time.Time
	// committedAt is when the entry was last written. The middleware commits
	// on every request, so the oldest is the one whose session has gone
	// longest without being used.
	committedAt time.Time
	// authenticated reports whether the snapshot carried a user, which is what
	// keeps an entry out of the eviction candidates.
	authenticated bool
}

// New returns a Store with optional overrides for the clock and the limits.
func New(opts ...Option) *Store {
	s := &Store{
		entries:    make(map[string]entry),
		maxEntries: defaultMaxEntries,
		maxBytes:   defaultMaxBytes,
		now:        time.Now,
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
		s.remove(id, e)
		return nil, nil
	}

	var snap session.Snapshot
	if err := json.Unmarshal(e.data, &snap); err != nil {
		return nil, fmt.Errorf("memstore: unmarshal snapshot: %w", err)
	}

	return &snap, nil
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

	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("memstore: marshal snapshot: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	bytes, ok := s.fits(snap.ID, len(data))
	if !ok {
		s.cull(now, snap.ID, len(data))

		if bytes, ok = s.fits(snap.ID, len(data)); !ok {
			return fmt.Errorf("memstore: store is full at %d entries and %d bytes, with no unauthenticated session to evict", len(s.entries), s.bytes)
		}
	}

	s.entries[snap.ID] = entry{
		data:          data,
		expiresAt:     now.Add(ttl),
		committedAt:   now,
		authenticated: snap.User != nil,
	}
	s.bytes = bytes

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.entries[id]; ok {
		s.remove(id, e)
	}

	return nil
}

// fits reports whether the store can take a snapshot of size bytes stored
// under id, and the byte total that write would leave behind. Replacing an
// entry trades its size for the new one rather than adding to the count.
func (s *Store) fits(id string, size int) (int, bool) {
	entries := len(s.entries)
	bytes := s.bytes + size

	if old, replacing := s.entries[id]; replacing {
		bytes -= len(old.data)
	} else {
		entries++
	}

	return bytes, entries <= s.maxEntries && bytes <= s.maxBytes
}

// cull frees room for a snapshot of size bytes stored under id. Expired
// entries go first, and live ones are touched only when that is not enough:
// evicting a session signs its owner out, so the store gives up the least
// recently committed unauthenticated entries and never an authenticated one.
// It removes a batch rather than the single entry needed, so the scan is paid
// once for many writes.
func (s *Store) cull(now time.Time, id string, size int) {
	type candidate struct {
		id          string
		committedAt time.Time
	}

	candidates := make([]candidate, 0, len(s.entries))

	for entryID, e := range s.entries {
		if !e.expiresAt.After(now) {
			s.remove(entryID, e)
			continue
		}
		// Evicting the entry being replaced frees nothing, since the write
		// already credits its size, and costs its owner the session when the
		// write is refused anyway.
		if !e.authenticated && entryID != id {
			candidates = append(candidates, candidate{id: entryID, committedAt: e.committedAt})
		}
	}

	if _, ok := s.fits(id, size); ok {
		return
	}

	slices.SortFunc(candidates, func(a, b candidate) int {
		return a.committedAt.Compare(b.committedAt)
	})

	batch := min(cullBatch, max(s.maxEntries/cullDivisor, 1))

	for i, c := range candidates {
		e, ok := s.entries[c.id]
		if !ok {
			continue
		}

		s.remove(c.id, e)

		// Past the batch, stop as soon as the write fits: the byte limit can
		// need more than the batch, and the entry limit never needs more.
		if i+1 >= batch {
			if _, ok := s.fits(id, size); ok {
				return
			}
		}
	}
}

// remove deletes the entry under id, keeping the byte total in step.
func (s *Store) remove(id string, e entry) {
	delete(s.entries, id)
	s.bytes -= len(e.data)
}
