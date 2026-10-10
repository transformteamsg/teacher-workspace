// Package memstore implements session.Store in memory. Entries are copied in
// and out, so callers share no state with the store or each other. The store
// holds a bounded number of entries and bytes, evicting entries to make room. It
// is intended for development and tests; production deployments should use a
// shared store.
package memstore

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	// defaultMaxEntries and defaultMaxBytes bound a store built without
	// limits. A session holds ~280 bytes signed out and ~1.5 KB with a sign-in
	// underway, so the byte limit is what binds first in the worst case.
	defaultMaxEntries = 50_000
	defaultMaxBytes   = 64 << 20

	// cullBatch and cullDivisor set how much a cull frees beyond the write that
	// triggered it, so the scan it costs is amortised over the writes that
	// follow. A cull frees this many entries even when the write needed one,
	// and more when the byte limit still does not fit.
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

// WithMaxEntries caps how many entries the store holds. A value below 1 keeps
// the default.
func WithMaxEntries(maxEntries int) Option {
	return func(s *Store) {
		if maxEntries > 0 {
			s.maxEntries = maxEntries
		}
	}
}

// WithMaxBytes caps the total size, in bytes, of the data the store holds. A
// value below 1 keeps the default.
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
	data        []byte
	expiresAt   time.Time
	committedAt time.Time
}

// New returns an in-memory Store. Unless an option sets them, it holds at most
// 50,000 entries and 64 MiB.
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
		s.remove(id, e)
		return nil, nil
	}

	return bytes.Clone(e.data), nil
}

// Commit implements [session.Store.Commit]. When the store is at its entry or
// byte limit, it evicts expired entries first, then the least recently
// committed, so an entry can be gone before its TTL elapses. When data is
// larger than the byte limit, it returns an error and leaves the entry under id
// unchanged.
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

	now := s.now()

	total, ok := s.fits(id, len(data))
	if !ok {
		s.cull(now, id, len(data))

		// Everything else can be evicted, so the only write that still does not
		// fit is one larger than the store itself.
		if total, ok = s.fits(id, len(data)); !ok {
			return fmt.Errorf("memstore: entry of %d bytes does not fit a store limited to %d", len(data), s.maxBytes)
		}
	}

	s.entries[id] = entry{
		data:        bytes.Clone(data),
		expiresAt:   now.Add(ttl),
		committedAt: now,
	}
	s.bytes = total

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("memstore: drop session: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.entries[id]; ok {
		s.remove(id, e)
	}

	return nil
}

// fits reports whether the store can take an entry of size bytes stored under
// id, and the byte total that write would leave behind. Replacing an entry
// trades its size for the new one rather than adding to the count.
func (s *Store) fits(id string, size int) (int, bool) {
	entries := len(s.entries)
	total := s.bytes + size

	if old, replacing := s.entries[id]; replacing {
		total -= len(old.data)
	} else {
		entries++
	}

	return total, entries <= s.maxEntries && total <= s.maxBytes
}

// cull frees room for an entry of size bytes stored under id. Expired entries
// go first, and live ones only when that is not enough, since evicting a live
// session signs its owner out. It removes a batch rather than the single entry
// needed, so the scan is paid once for many writes.
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
		// already credits its size.
		if entryID != id {
			candidates = append(candidates, candidate{id: entryID, committedAt: e.committedAt})
		}
	}

	if _, ok := s.fits(id, size); ok {
		return
	}

	// The session middleware commits on every request, so the least recently
	// committed entry is the session that has gone longest without being used.
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
