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

	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
	"github.com/transformteamsg/teacher-workspace/server/internal/session"
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
// through [SessionFromContext], and updates the store to match it once the
// request is handled.
//
// The session is the one identified by the request's session cookie. A new
// session is created when the cookie is missing, names no stored session, or
// names one that cannot be decoded.
//
// The store is updated just before the response is first written, or after the
// handler returns if it writes nothing, so changes made to the session after
// the first write are lost. The update goes ahead even if the client has
// disconnected:
//   - A destroyed session is removed, and the session cookie is expired.
//   - A session that is no longer in the store, such as one destroyed by
//     another request, is not saved, and the session cookie is left as it is,
//     unless the request changed its ID.
//   - Any other session is saved to expire after [SessionOptions.DefaultTTL],
//     or [SessionOptions.AuthenticatedTTL] when a user is attached, and the
//     session cookie carries its ID for as long. If its ID has changed, the
//     superseded entry under its previous ID is removed on a best-effort basis.
//
// The handler's response carries Cache-Control: no-store, replacing any value
// the handler set.
//
// If the store fails to load the session, the request goes no further and the
// response is a 500. If the session cannot be saved, or a destroyed session
// cannot be removed, the handler's response is replaced with a 500. If the
// client disconnects before the session loads, nothing is written.
func Session(store session.Store, opts SessionOptions) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := LoggerFromContext(r.Context())

			var id string
			if c, err := r.Cookie(opts.Name); err == nil {
				id = c.Value
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

			sw := &sessionResponseWriter{
				ResponseWriter: w,
				initialHeader:  w.Header().Clone(),
				logger:         logger,
				finalise: func(h http.Header) error {
					// Finalise without cancellation: the store must reflect the handled
					// request even if the client disconnects.
					ctx := context.WithoutCancel(r.Context())

					h.Set("Cache-Control", "no-store")

					ttl := opts.DefaultTTL
					if sess.IsAuthenticated() {
						ttl = opts.AuthenticatedTTL
					}

					switch err := session.Commit(ctx, store, sess, ttl); {
					case errors.Is(err, session.ErrNotFound):
						// Another request destroyed the session while this one ran, and
						// may already have sent its own cookie. Saving the session would
						// sign the client back in, and any cookie sent here, even an
						// expired one, would replace a cookie that a later request may
						// have set since.
						return nil
					case errors.Is(err, session.ErrSupersededNotDeleted):
						// The session is committed, and the superseded entry expires by
						// TTL. A destroyed session gets no such leeway: a client still
						// holding its ID would stay logged in, so failing to remove it
						// fails the request.
						logger.Error("failed to delete superseded session", "err", err)
					case err != nil:
						logger.Error("failed to commit session", "err", err)
						return err
					}

					cookie := &http.Cookie{
						Name:     opts.Name,
						Path:     "/",
						Secure:   opts.Secure,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					}
					if sess.IsDestroyed() {
						// A zero MaxAge omits the attribute rather than expiring the
						// cookie.
						cookie.MaxAge = -1
					} else {
						cookie.Value = sess.ID()
						cookie.MaxAge = int(ttl.Seconds())
					}

					h.Add("Set-Cookie", cookie.String())

					return nil
				},
			}

			ctx := WithSession(r.Context(), sess)
			next.ServeHTTP(sw, r.WithContext(ctx))

			// Runs finalise for a response that was never written.
			sw.finaliseOnce()
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

// sessionResponseWriter runs finalise once, just before the response headers
// are sent. If finalise fails, it sends a 500 in place of the response being
// written, with only the headers the response had when the writer was created,
// and discards any later writes.
type sessionResponseWriter struct {
	http.ResponseWriter

	// finalise is the function run with the response headers just before they
	// are sent, which it may add to. It logs any error it returns.
	finalise func(h http.Header) error

	// initialHeader holds the response headers as they were when the writer was
	// created.
	initialHeader http.Header

	logger      *slog.Logger
	once        sync.Once
	finaliseErr error
}

// finaliseOnce runs finalise on its first call and does nothing after. If
// finalise fails, it sends a 500 in place of the response being written.
func (rw *sessionResponseWriter) finaliseOnce() {
	rw.once.Do(func() {
		err := rw.finalise(rw.Header())
		if err == nil {
			return
		}

		rw.finaliseErr = err

		// Reset to the headers from when the writer was created. Any added since
		// were meant for a response that will not be sent, and some, such as
		// Content-Length, would corrupt the 500.
		h := rw.Header()
		clear(h)
		maps.Copy(h, rw.initialHeader)

		httputil.RenderPlain(rw.ResponseWriter, rw.logger, http.StatusInternalServerError)
	})
}

// WriteHeader runs finalise if status is final, then sends status. If finalise
// failed, the 500 takes the place of status.
func (rw *sessionResponseWriter) WriteHeader(status int) {
	// A 1xx other than 101 leaves the final headers unsent.
	if status >= 200 || status == http.StatusSwitchingProtocols {
		rw.finaliseOnce()
	}
	if rw.finaliseErr != nil {
		return
	}

	rw.ResponseWriter.WriteHeader(status)
}

// Write runs finalise, then writes b. If finalise failed, it discards b and
// returns finalise's error.
func (rw *sessionResponseWriter) Write(b []byte) (int, error) {
	rw.finaliseOnce()
	if rw.finaliseErr != nil {
		return 0, rw.finaliseErr
	}

	return rw.ResponseWriter.Write(b)
}

// Flush runs finalise, then flushes the response. If finalise failed, it
// returns without flushing.
func (rw *sessionResponseWriter) Flush() {
	rw.finaliseOnce()
	if rw.finaliseErr != nil {
		return
	}

	_ = http.NewResponseController(rw.ResponseWriter).Flush()
}

// Hijack runs finalise, then hands over the connection. If finalise failed, it
// returns finalise's error.
func (rw *sessionResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw.finaliseOnce()
	if rw.finaliseErr != nil {
		return nil, nil, rw.finaliseErr
	}

	return http.NewResponseController(rw.ResponseWriter).Hijack()
}

// Unwrap returns the underlying ResponseWriter, so [http.ResponseController]
// can reach the methods sessionResponseWriter does not override.
func (rw *sessionResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
