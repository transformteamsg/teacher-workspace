package middleware

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/session"
)

type ctxKeySession struct{}

// SessionOptions configures the [Session] middleware.
type SessionOptions struct {
	// Name is the session cookie name. It must be a valid cookie name.
	Name string

	// DefaultTTL is how long an unauthenticated session lasts without a
	// request before it expires. It must be at least one second.
	DefaultTTL time.Duration

	// AuthenticatedTTL is how long an authenticated session lasts without a
	// request before it expires. It must be at least one second.
	AuthenticatedTTL time.Duration

	// Secure restricts the session cookie to HTTPS.
	Secure bool
}

// Session is a middleware that gives each request a session, available
// through [SessionFromContext], and saves it once the handler responds.
//
// The session is the one identified by the request's session cookie. A new
// session is created when the cookie is missing, names no stored session, or
// names one that cannot be decoded.
//
// The session is saved before the first write of the response, so handlers
// must change it before writing: changes made after the first write are not
// saved. It is saved even if the client disconnects while the handler runs.
// Every response carries the session cookie and Cache-Control: no-store. If
// the session's ID has changed, the cookie carries the new ID, and the
// session under the old ID is removed on a best-effort basis.
//
// If the store fails to load the session, or the session cannot be saved, the
// response is a 500. If the client disconnects before the session loads,
// nothing is written.
func Session(store session.Store, opts SessionOptions) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := LoggerFromContext(r.Context())

			var id string
			cookie, err := r.Cookie(opts.Name)
			if err == nil {
				id = cookie.Value
			}

			sess, err := session.Load(r.Context(), store, id)
			if err != nil {
				switch {
				case errors.Is(err, context.Canceled):
					// Cancellation means the client disconnected, which is expected.
					// Logging it would only add noise, and the response would be lost.
					return
				case errors.Is(err, session.ErrUndecodable):
					logger.Warn("discarding undecodable session", "err", err)
					sess = session.New()
				default:
					logger.Error("failed to load session", "err", err)
					httputil.RenderPlain(w, logger, http.StatusInternalServerError)
					return
				}
			}

			initialID := sess.ID()

			sw := &sessionResponseWriter{
				ResponseWriter: w,
				initialHeader:  w.Header().Clone(),
				logger:         logger,
				commit: func(h http.Header) error {
					ttl := opts.DefaultTTL
					if sess.IsAuthenticated() {
						ttl = opts.AuthenticatedTTL
					}

					// Save even if the client disconnects: the request has already
					// been handled, and the store must reflect it.
					ctx := context.WithoutCancel(r.Context())

					if err := session.Save(ctx, store, sess, ttl); err != nil {
						return err
					}

					h.Set("Cache-Control", "no-store")
					h.Add("Set-Cookie", (&http.Cookie{
						Name:     opts.Name,
						Value:    sess.ID(),
						Path:     "/",
						MaxAge:   int(ttl.Seconds()),
						Secure:   opts.Secure,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					}).String())

					// Once the ID changes, the entry under the old one is stale.
					// Dropping it is best effort: it expires by TTL otherwise.
					if sess.ID() != initialID {
						if err := store.Drop(ctx, initialID); err != nil {
							logger.Error("failed to drop superseded session", "err", err)
						}
					}

					return nil
				},
			}

			ctx := WithSession(r.Context(), sess)
			next.ServeHTTP(sw, r.WithContext(ctx))

			// Covers handlers that never write; otherwise the save has already run.
			sw.commitOnce()
		})
	}
}

// SessionFromContext retrieves the session the [Session] middleware attached
// to ctx. The returned boolean reports whether a session was present.
func SessionFromContext(ctx context.Context) (*session.Session, bool) {
	sess, ok := ctx.Value(ctxKeySession{}).(*session.Session)
	return sess, ok
}

// WithSession attaches sess to ctx. Only the [Session] middleware and tests
// should call it.
func WithSession(ctx context.Context, sess *session.Session) context.Context {
	return context.WithValue(ctx, ctxKeySession{}, sess)
}

// sessionResponseWriter saves the session and sets the session cookie just
// before the response headers are sent. If the session could not be saved, it
// sends a 500 in place of the response being written and discards any later
// writes.
type sessionResponseWriter struct {
	http.ResponseWriter

	// commit is called once, before the response headers are sent, with the
	// headers it may add to. A non-nil error replaces the response with a 500.
	commit func(h http.Header) error

	// initialHeader holds the response headers as they were when the writer was
	// created.
	initialHeader http.Header

	logger    *slog.Logger
	once      sync.Once
	commitErr error
}

// commitOnce runs commit on its first call and does nothing after. If commit
// fails, it sends a 500 in place of the response being written.
func (rw *sessionResponseWriter) commitOnce() {
	rw.once.Do(func() {
		err := rw.commit(rw.Header())
		if err == nil {
			return
		}

		rw.logger.Error("failed to save session", "err", err)
		rw.commitErr = err

		// Reset to the headers from when the writer was created. Any added since
		// were meant for a response that will not be sent, and some, such as
		// Content-Length, would corrupt the 500.
		h := rw.Header()
		clear(h)
		maps.Copy(h, rw.initialHeader)

		httputil.RenderPlain(rw.ResponseWriter, rw.logger, http.StatusInternalServerError)
	})
}

// WriteHeader saves the session if status is final, then sends status. If the
// session could not be saved, it sends a 500 in place of status.
func (rw *sessionResponseWriter) WriteHeader(status int) {
	// A 1xx other than 101 leaves the final headers unsent.
	if status >= 200 || status == http.StatusSwitchingProtocols {
		rw.commitOnce()
	}
	if rw.commitErr != nil {
		return
	}

	rw.ResponseWriter.WriteHeader(status)
}

// Write saves the session, then writes b. If the session could not be saved,
// it discards b and returns the save error.
func (rw *sessionResponseWriter) Write(b []byte) (int, error) {
	rw.commitOnce()
	if rw.commitErr != nil {
		return 0, rw.commitErr
	}

	return rw.ResponseWriter.Write(b)
}

// Flush saves the session, then flushes the response. If the session could
// not be saved, it returns without flushing.
func (rw *sessionResponseWriter) Flush() {
	rw.commitOnce()
	if rw.commitErr != nil {
		return
	}

	_ = http.NewResponseController(rw.ResponseWriter).Flush()
}

// Hijack saves the session, then hands over the connection. If the session
// could not be saved, it returns the save error.
func (rw *sessionResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw.commitOnce()
	if rw.commitErr != nil {
		return nil, nil, rw.commitErr
	}

	return http.NewResponseController(rw.ResponseWriter).Hijack()
}

// Unwrap returns the underlying ResponseWriter, so [http.ResponseController]
// can reach the methods sessionResponseWriter does not override.
func (rw *sessionResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
