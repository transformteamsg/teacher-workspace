// Package session provides a typed HTTP session abstraction with a pluggable
// store. A Session is owned by one request goroutine and is not safe for
// concurrent use.
package session

import (
	"context"
	"time"

	"github.com/String-sg/teacher-workspace/server/pkg/random"
)

// Store persists session snapshots keyed by their ID.
type Store interface {
	// Prepare returns the snapshot stored under id, or (nil, nil) when no
	// entry exists or when id is empty.
	Prepare(ctx context.Context, id string) (*Snapshot, error)

	// Commit writes snap with the given TTL. Returns an error when snap is
	// nil, snap.ID is empty, or ttl is non-positive.
	Commit(ctx context.Context, snap *Snapshot, ttl time.Duration) error

	// Drop removes the entry stored under id. It is a no-op when id is
	// absent or empty.
	Drop(ctx context.Context, id string) error
}

// User identifies the authenticated principal attached to a session.
type User struct {
	Email string `json:"email"`
}

// Snapshot is the wire/storage form of a Session. The Data map is shared with
// the Session that produced (or consumed) it.
type Snapshot struct {
	ID        string         `json:"id"`
	CSRFToken string         `json:"csrf_token"`
	User      *User          `json:"user,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// Session holds the per-request authentication and CSRF state. Sessions are
// owned by a single request goroutine and must not be shared across goroutines.
type Session struct {
	id        string
	csrfToken string
	user      *User
	data      map[string]any
}

// New returns a fresh unauthenticated Session with a freshly generated ID and
// CSRF token.
func New() *Session {
	return &Session{
		id:        newToken(),
		csrfToken: newToken(),
	}
}

// FromSnapshot rehydrates a Session from snap. The returned session shares its
// Data map with the snapshot. Panics when snap is nil.
func FromSnapshot(snap *Snapshot) *Session {
	if snap == nil {
		panic("session: FromSnapshot called with nil snapshot")
	}

	return &Session{
		id:        snap.ID,
		csrfToken: snap.CSRFToken,
		user:      snap.User,
		data:      snap.Data,
	}
}

// Snapshot returns the current state of the session for storage. The returned
// snapshot shares its Data map with the session.
func (s *Session) Snapshot() *Snapshot {
	return &Snapshot{
		ID:        s.id,
		CSRFToken: s.csrfToken,
		User:      s.user,
		Data:      s.data,
	}
}

// ID returns the session's opaque identifier, used as the session cookie value
// and storage key.
func (s *Session) ID() string {
	return s.id
}

// CSRFToken returns the session's CSRF token, valid for the session's lifetime.
func (s *Session) CSRFToken() string {
	return s.csrfToken
}

// User returns the authenticated user, or nil if the session is unauthenticated.
func (s *Session) User() *User {
	return s.user
}

// IsAuthenticated reports whether a user has been attached to the session.
func (s *Session) IsAuthenticated() bool {
	return s.user != nil
}

// Get returns the value stored under key and whether it was present.
func (s *Session) Get(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

// Set stores val under key, lazily initialising the underlying map on first use.
func (s *Session) Set(key string, val any) {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	s.data[key] = val
}

// Delete removes key from the session. It is a no-op when the key is absent or
// when no value has ever been set.
func (s *Session) Delete(key string) {
	delete(s.data, key)
}

// SetUser attaches u to the session. Transitioning from unauthenticated to
// authenticated rotates the ID and CSRF token to defend against session
// fixation, and discards the data so nothing seeded into the anonymous session
// crosses the privilege boundary. Read a value before the call and set it after
// to carry it across. Subsequent SetUser calls (auth->auth) change nothing but
// the user.
func (s *Session) SetUser(u *User) {
	if s.user == nil {
		s.Rotate()
		clear(s.data)
	}
	s.user = u
}

// Rotate regenerates the session ID and CSRF token while preserving the
// authenticated user and data. The middleware is responsible for cleaning up
// the old store entry after rotation.
func (s *Session) Rotate() {
	s.id = newToken()
	s.csrfToken = newToken()
}

// newToken returns a 32-character base58 alphanumeric token, used for both
// session IDs and CSRF tokens.
func newToken() string {
	return random.Base58(32)
}
