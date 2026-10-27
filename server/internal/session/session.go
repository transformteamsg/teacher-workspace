// Package session provides HTTP sessions persisted in a pluggable Store.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/transformteamsg/teacher-workspace/server/pkg/random"
)

// tokenLength is the length, in characters, of session IDs and of CSRF tokens
// before masking.
const tokenLength = 32

// ErrUndecodable is wrapped by the error [Load] returns when the entry it reads
// is not a session stored under the requested ID.
var ErrUndecodable = errors.New("session: undecodable entry")

// ErrNotFound is returned by [Commit] when the entry it would replace is no
// longer in store.
var ErrNotFound = errors.New("session: entry not found")

// ErrSupersededNotDeleted is wrapped by the error [Commit] returns when it has
// written a session under a new ID but cannot remove the superseded entry under
// the session's previous ID. The session is committed regardless.
var ErrSupersededNotDeleted = errors.New("session: superseded entry not deleted")

// Session is one client's state across requests. Obtain one with New or Load;
// the zero value is not usable. A Session is not safe for concurrent use.
type Session struct {
	id        string
	csrfToken string
	user      *User
	data      map[string]any
	destroyed bool

	// storedID is the ID the session was last loaded or committed under, or
	// empty if it has been neither.
	storedID string
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
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Email      string   `json:"email"`
	Role       string   `json:"role"`
	SchoolCode string   `json:"school_code"`
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
// session stored under id, or the error from store when store fails.
func Load(ctx context.Context, store Store, id string) (*Session, error) {
	data, err := store.Get(ctx, id)
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
		return nil, fmt.Errorf("%w: holds a different session ID", ErrUndecodable)
	}
	if snap.CSRFToken == "" {
		return nil, fmt.Errorf("%w: missing CSRF token", ErrUndecodable)
	}

	return &Session{
		id:        snap.ID,
		csrfToken: snap.CSRFToken,
		user:      snap.User,
		data:      snap.Data,
		storedID:  snap.ID,
	}, nil
}

// Commit updates store to match sess:
//   - A destroyed session is removed.
//   - Any other session is written under its ID, to expire after ttl. If its
//     ID has changed since it was loaded or last committed, the superseded
//     entry under its previous ID is then removed.
//
// It returns these errors:
//   - [ErrNotFound], writing nothing, if sess's ID is unchanged since it was
//     loaded or last committed but its entry is no longer in store.
//     A session whose ID has changed is written even then.
//   - [ErrSupersededNotDeleted], wrapped with the store's error, if sess is
//     written under its new ID but the superseded entry cannot be removed.
//     sess is committed regardless, so a later Commit sees its ID as
//     unchanged.
//   - An error, writing nothing, if a value set with [Session.Set] cannot be
//     encoded as JSON or sess's ID is taken by another session.
//   - The error from store if any other store call fails.
func Commit(ctx context.Context, store Store, sess *Session, ttl time.Duration) error {
	if sess.destroyed {
		return store.Delete(ctx, sess.storedID)
	}

	data, err := json.Marshal(snapshot{
		ID:        sess.id,
		CSRFToken: sess.csrfToken,
		User:      sess.user,
		Data:      sess.data,
	})
	if err != nil {
		return fmt.Errorf("session: unencodable value: %w", err)
	}

	if sess.id != sess.storedID {
		created, err := store.Create(ctx, sess.id, data, ttl)
		if err != nil {
			return err
		}
		if !created {
			return errors.New("session: ID already taken")
		}

		supersededID := sess.storedID
		sess.storedID = sess.id

		if err := store.Delete(ctx, supersededID); err != nil {
			return fmt.Errorf("%w: %w", ErrSupersededNotDeleted, err)
		}

		return nil
	}

	updated, err := store.Update(ctx, sess.id, data, ttl)
	if err != nil {
		return err
	}
	if !updated {
		return ErrNotFound
	}

	return nil
}

// ID returns the session's opaque identifier, used as the session cookie value
// and storage key.
func (s *Session) ID() string {
	return s.id
}

// CSRFToken returns a URL-safe CSRF token for the session. Each call returns a
// different token.
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

// Destroy ends the session, detaching its user and discarding its data.
// [Commit] then removes it from the store.
func (s *Session) Destroy() {
	s.user = nil
	clear(s.data)
	s.destroyed = true
}

// IsDestroyed reports whether [Session.Destroy] has been called.
func (s *Session) IsDestroyed() bool {
	return s.destroyed
}
