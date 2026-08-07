// Package valkeystore implements session.Store backed by Valkey. Snapshots are
// JSON-encoded under a caller-configurable key prefix and expire server-side
// when their TTL elapses, so no sweeper is needed. It is intended for
// deployments where session state must outlive a single process and be shared
// across instances.
package valkeystore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideconfig "github.com/valkey-io/valkey-glide/go/v2/config"
	glideopts "github.com/valkey-io/valkey-glide/go/v2/options"

	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// Option configures a Store.
type Option func(*Store)

// WithPrefix sets the storage-key prefix. Default is "session:".
func WithPrefix(prefix string) Option {
	return func(s *Store) { s.prefix = prefix }
}

// Store is a Valkey-backed session.Store. It does not own the client it is
// given, so closing the client is the caller's responsibility.
type Store struct {
	client *glide.Client
	prefix string
}

// New returns a Store that uses client to talk to Valkey. The default key
// prefix is "session:"; override with WithPrefix.
func New(client *glide.Client, opts ...Option) *Store {
	s := &Store{client: client, prefix: "session:"}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Dial connects to the Valkey server addressed by u and verifies the connection
// before returning, so an unreachable server surfaces at startup rather than on
// the first request. Bound the wait by cancelling ctx. The caller owns the
// returned client and must Close it. Errors carry the host and port only, never
// the URL's credentials.
func Dial(ctx context.Context, u *url.URL) (*glide.Client, error) {
	cfg, err := clientConfig(u)
	if err != nil {
		return nil, err
	}

	// glide.NewClient takes no context, so ctx alone would bound only the ping
	// below and leave the connection attempt on glide's own default. Handing
	// the deadline over as an explicit connection timeout is what makes the
	// caller's bound apply to an unreachable server, which is the case that
	// matters at startup.
	if deadline, ok := ctx.Deadline(); ok {
		cfg = cfg.WithAdvancedConfiguration(
			glideconfig.NewAdvancedClientConfiguration().WithConnectionTimeout(time.Until(deadline)),
		)
	}

	client, err := glide.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("valkeystore: connect to %s: %w", u.Host, err)
	}

	// NewClient can report success before the connection is usable, so the ping
	// is what actually establishes reachability.
	if _, err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("valkeystore: ping %s: %w", u.Host, err)
	}

	return client, nil
}

// connOptions is the parsed form of a Valkey URL. It exists so the URL contract
// can be tested without reaching into the glide configuration, whose fields are
// unexported.
type connOptions struct {
	host     string
	port     int
	useTLS   bool
	username string
	password string
	// database is the logical database index, or nil when the URL has no path.
	database *int
}

// parseURL splits u into connection options. TLS comes from the tls query
// parameter rather than the scheme, matching the URL format the platform stores
// in Secrets Manager.
func parseURL(u *url.URL) (connOptions, error) {
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return connOptions{}, fmt.Errorf("valkeystore: parse port from %q: %w", u.Host, err)
	}

	opts := connOptions{
		host:   u.Hostname(),
		port:   port,
		useTLS: u.Query().Get("tls") == "true",
	}

	if u.User != nil {
		opts.username = u.User.Username()
		opts.password, _ = u.User.Password()
	}

	if db := strings.Trim(u.Path, "/"); db != "" {
		id, err := strconv.Atoi(db)
		if err != nil {
			return connOptions{}, fmt.Errorf("valkeystore: parse database id from path %q: %w", u.Path, err)
		}
		opts.database = &id
	}

	return opts, nil
}

// clientConfig translates u into a glide client configuration.
func clientConfig(u *url.URL) (*glideconfig.ClientConfiguration, error) {
	opts, err := parseURL(u)
	if err != nil {
		return nil, err
	}

	cfg := glideconfig.NewClientConfiguration().
		WithAddress(&glideconfig.NodeAddress{Host: opts.host, Port: opts.port}).
		WithUseTLS(opts.useTLS)

	if opts.username != "" || opts.password != "" {
		cfg = cfg.WithCredentials(glideconfig.NewServerCredentials(opts.username, opts.password))
	}
	if opts.database != nil {
		cfg = cfg.WithDatabaseId(*opts.database)
	}

	return cfg, nil
}

// Prepare implements [session.Store.Prepare]. An entry that cannot be decoded
// is logged and reported as absent, so a session written by an incompatible
// build starts over rather than failing every request until its TTL elapses.
func (s *Store) Prepare(ctx context.Context, id string) (*session.Snapshot, error) {
	if id == "" {
		return nil, nil
	}

	result, err := s.client.Get(ctx, s.prefix+id)
	if err != nil {
		return nil, fmt.Errorf("valkeystore: prepare session: %w", err)
	}
	// A key that has passed its TTL reads as absent, so expiry needs no special
	// handling here.
	if result.IsNil() {
		return nil, nil
	}

	var snap session.Snapshot
	if err := json.Unmarshal([]byte(result.Value()), &snap); err != nil {
		// An entry that cannot be decoded outlives the process that wrote it, so
		// surfacing an error would fail every request carrying this ID until the
		// TTL elapses, with no cookie issued to escape it. Starting a fresh
		// session is what expiry already does with an unusable entry.
		slog.ErrorContext(ctx, "valkeystore: discarding undecodable session", "err", err)
		return nil, nil
	}

	return &snap, nil
}

// Commit implements [session.Store.Commit].
func (s *Store) Commit(ctx context.Context, snap *session.Snapshot, ttl time.Duration) error {
	if snap == nil {
		return fmt.Errorf("valkeystore: snap must be non-nil")
	}
	if snap.ID == "" {
		return fmt.Errorf("valkeystore: snap.ID must be non-empty")
	}
	if ttl <= 0 {
		return fmt.Errorf("valkeystore: ttl must be positive, got %v", ttl)
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("valkeystore: marshal snapshot: %w", err)
	}

	if _, err := s.client.SetWithOptions(ctx, s.prefix+snap.ID, string(data), glideopts.SetOptions{
		Expiry: glideopts.NewExpiryIn(ttl),
	}); err != nil {
		return fmt.Errorf("valkeystore: commit session: %w", err)
	}

	return nil
}

// Drop implements [session.Store.Drop].
func (s *Store) Drop(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}

	if _, err := s.client.Del(ctx, []string{s.prefix + id}); err != nil {
		return fmt.Errorf("valkeystore: drop session: %w", err)
	}

	return nil
}
