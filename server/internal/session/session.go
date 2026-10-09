// Package session provides HTTP sessions persisted in a pluggable Store.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/String-sg/teacher-workspace/server/pkg/random"
)

// Store persists encoded sessions by ID until their TTL elapses.
// Implementations must be safe for concurrent use. If the context is already
// done when a method is called, the method returns an error wrapping the
// context's error and leaves the entries unchanged.
type Store interface {
	// Prepare returns the bytes last committed under id, or (nil, nil) when id
	// is empty, has no entry, or its entry has expired.
	Prepare(ctx context.Context, id string) ([]byte, error)

	// Commit stores data under id for ttl, replacing any existing entry. When id
	// is empty or ttl is not positive, it returns an error and leaves the entries
	// unchanged.
	Commit(ctx context.Context, id string, data []byte, ttl time.Duration) error

	// Drop removes the entry under id. It is a no-op when id is empty or has no
	// entry.
	Drop(ctx context.Context, id string) error
}

// tokenLength is the length, in characters, of session IDs and of CSRF tokens
// before masking.
const tokenLength = 32

// ErrUndecodable is wrapped by the error [Load] returns when the entry it reads
// is not a session stored under the requested ID.
var ErrUndecodable = errors.New("session: undecodable entry")

// Session is one client's state across requests. Obtain one with New or Load;
// the zero value is not usable. A Session is not safe for concurrent use.
type Session struct {
	id        string
	csrfToken string
	user      *User
	data      map[string]any
}

// snapshot is the stored form of a Session.
type snapshot struct {
	ID        string         `json:"id"`
	CSRFToken string         `json:"csrf_token"`
	User      *User          `json:"user,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// User is the authenticated principal of a Session.
type User struct {
	Email string `json:"email"`
	// Role is the staff member's Edupass base role, without its location and
	// environment prefix, such as "ROLE_TEACHER".
	Role string `json:"role"`
	// Attributes contains the staff member's Edupass attributes, without their
	// location and environment prefix, such as "ATTR_PG_ADMIN", unranked and in
	// the order Edupass issued them.
	Attributes []string `json:"attributes"`
}

// New returns a new unauthenticated Session.
func New() *Session {
	return &Session{
		id:        random.Base58(tokenLength),
		csrfToken: random.Base58(tokenLength),
	}
}

// Load returns the session stored under id in store, or a new session when
// there is none.
//
// It returns an error wrapping [ErrUndecodable] when the entry is not a
// session stored under id, or the error from store.
func Load(ctx context.Context, store Store, id string) (*Session, error) {
	data, err := store.Prepare(ctx, id)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return New(), nil
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUndecodable, err)
	}
	if snap.ID != id {
		return nil, fmt.Errorf("%w: ID mismatch", ErrUndecodable)
	}
	if snap.CSRFToken == "" {
		return nil, fmt.Errorf("%w: no CSRF token", ErrUndecodable)
	}

	return &Session{
		id:        snap.ID,
		csrfToken: snap.CSRFToken,
		user:      snap.User,
		data:      snap.Data,
	}, nil
}

// Save writes sess to store under its ID, to expire after ttl. An entry stored
// under an ID the session had before [Session.SetUser] stays in store until it
// expires or is dropped.
//
// It returns an error when a value set with [Session.Set] cannot be encoded as
// JSON, or the error from store.
func Save(ctx context.Context, store Store, sess *Session, ttl time.Duration) error {
	data, err := json.Marshal(snapshot{
		ID:        sess.id,
		CSRFToken: sess.csrfToken,
		User:      sess.user,
		Data:      sess.data,
	})
	if err != nil {
		return fmt.Errorf("session: encode: %w", err)
	}

	return store.Commit(ctx, sess.id, data, ttl)
}

// ID returns the session's opaque identifier, used as the session cookie value
// and storage key.
func (s *Session) ID() string {
	return s.id
}

// CSRFToken returns a URL-safe CSRF token for the session. Each call returns a
// different token, and [Session.VerifyCSRFToken] accepts all of them until
// [Session.SetUser] is called.
func (s *Session) CSRFToken() string {
	return maskToken(s.csrfToken)
}

// VerifyCSRFToken reports whether token was returned by [Session.CSRFToken]
// and has not been invalidated by a later [Session.SetUser] call. It reports
// false for an empty or malformed token, and does not leak the session's token
// through timing.
func (s *Session) VerifyCSRFToken(token string) bool {
	return verifyToken(s.csrfToken, token)
}

// User returns the user attached to the session, and reports whether there is
// one.
func (s *Session) User() (User, bool) {
	if s.user == nil {
		return User{}, false
	}

	return *s.user, true
}

// IsAuthenticated reports whether a user has been attached to the session.
func (s *Session) IsAuthenticated() bool {
	return s.user != nil
}

// Get returns the value stored under key, and reports whether it is present and
// of type T. On a loaded session, numbers are of type float64 and structs of
// type map[string]any, so read them back as those types.
func (s *Session) Get[T any](key string) (T, bool) {
	v, ok := s.data[key].(T)
	return v, ok
}

// GetAndDelete returns what [Session.Get] returns for key, and removes key from
// the session. The key is removed even when its value is not of type T.
func (s *Session) GetAndDelete[T any](key string) (T, bool) {
	v, ok := s.data[key].(T)
	delete(s.data, key)
	return v, ok
}

// Set stores val under key. val must be encodable as JSON.
func (s *Session) Set(key string, val any) {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	s.data[key] = val
}

// Delete removes the value stored under key, if any.
func (s *Session) Delete(key string) {
	delete(s.data, key)
}

// SetUser attaches u to the session, replacing any user already attached.
//
// To defend against session fixation, it also gives the session a new ID,
// invalidates every token returned by [Session.CSRFToken] so far, and discards
// the session's data.
func (s *Session) SetUser(u User) {
	s.id = random.Base58(tokenLength)
	s.csrfToken = random.Base58(tokenLength)
	s.user = &u
	clear(s.data)
}
