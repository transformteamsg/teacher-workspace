package session

import (
	"context"
	"time"
)

// Store persists encoded sessions by ID until their TTL elapses.
// Implementations must be safe for concurrent use. If the context is already
// done when a method is called, the method returns an error wrapping the
// context's error and changes nothing.
type Store interface {
	// Get returns the data stored under id, or (nil, nil) when id is empty or
	// has no entry.
	Get(ctx context.Context, id string) ([]byte, error)

	// Create stores data under id for ttl. It returns false if id already has
	// an entry. It returns an error if id or data is empty or ttl is not
	// positive.
	Create(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error)

	// Update replaces the entry under id with data for ttl. It returns false if
	// id has no entry. It returns an error if id or data is empty or ttl is not
	// positive.
	Update(ctx context.Context, id string, data []byte, ttl time.Duration) (bool, error)

	// Delete removes the entry under id. It is a no-op when id is empty or has
	// no entry.
	Delete(ctx context.Context, id string) error
}
