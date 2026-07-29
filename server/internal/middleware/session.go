package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/session"
)

type ctxKeySession struct{}

// sessionResponseWriter runs a save hook just before the response headers
// reach the wire, so the Set-Cookie header reflects the session state the
// handler left behind rather than its pre-handler state.
type sessionResponseWriter struct {
	http.ResponseWriter

	once sync.Once
	save func(w http.ResponseWriter)
}

// saveOnce runs the save hook at most once per request.
func (rw *sessionResponseWriter) saveOnce() {
	rw.once.Do(func() { rw.save(rw.ResponseWriter) })
}

// WriteHeader saves the session, then forwards to the underlying ResponseWriter.
func (rw *sessionResponseWriter) WriteHeader(status int) {
	rw.saveOnce()
	rw.ResponseWriter.WriteHeader(status)
}

// Write saves the session before the first body byte, which flushes the
// headers with it.
func (rw *sessionResponseWriter) Write(b []byte) (int, error) {
	rw.saveOnce()
	return rw.ResponseWriter.Write(b)
}

// Flush saves the session first, since a flush commits the headers the same
// way a write does.
func (rw *sessionResponseWriter) Flush() {
	rw.saveOnce()
	_ = http.NewResponseController(rw.ResponseWriter).Flush()
}

// Unwrap returns the underlying ResponseWriter so http.ResponseController can
// reach optional interfaces (Hijack, SetWriteDeadline, etc.) on the real writer.
func (rw *sessionResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

type SessionOptions struct {
	// Name is the session cookie name.
	Name string

	// DefaultTTL is how long an unauthenticated session is retained in the
	// store before it expires.
	DefaultTTL time.Duration

	// AuthenticatedTTL is how long an authenticated session is retained in the
	// store before it expires.
	AuthenticatedTTL time.Duration

	// Secure marks the session cookie as Secure so it is only sent over
	// HTTPS. Enable in production; disable for local HTTP development.
	Secure bool
}

// Session is a middleware that loads the session identified by the request
// cookie into the context, then commits the session to the store and refreshes
// the cookie once the handler is done. Handlers never touch the cookie.
//
// The save runs on the handler's first write, so a session must be mutated
// before anything is written to the response.
func Session(store session.Store, opts SessionOptions) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var id string

			cookie, err := r.Cookie(opts.Name)
			if err == nil {
				id = cookie.Value
			}

			logger := LoggerFromContext(r.Context())

			snap, err := store.Prepare(r.Context(), id)
			if err != nil {
				logger.Error("failed to prepare session", "err", err)
				httputil.RenderPlain(w, logger, http.StatusInternalServerError)
				return
			}

			var sess *session.Session
			if snap != nil {
				sess = session.FromSnapshot(snap)
			} else {
				sess = session.New()
			}

			// Rotation is the only thing that changes the ID, so a mismatch at
			// save time means the entry it superseded needs dropping.
			initialID := sess.ID()

			rw := &sessionResponseWriter{ResponseWriter: w}
			rw.save = func(w http.ResponseWriter) {
				ttl := opts.DefaultTTL
				if sess.IsAuthenticated() {
					ttl = opts.AuthenticatedTTL
				}

				ctx := context.WithoutCancel(r.Context())

				// A failed commit is logged, not surfaced: the handler's
				// response stands.
				if err := store.Commit(ctx, sess.Snapshot(), ttl); err != nil {
					logger.Error("failed to commit session", "err", err)
					return
				}

				// Refresh the cookie every request so its Max-Age slides with
				// the store TTL. After Commit, so the client is never handed an
				// ID the store never took.
				http.SetCookie(w, &http.Cookie{
					Name:     opts.Name,
					Value:    sess.ID(),
					Path:     "/",
					MaxAge:   int(ttl.Seconds()),
					Secure:   opts.Secure,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})

				// Clearing the superseded entry is best effort. A failure leaves
				// it to expire by TTL, which is better than withholding the
				// cookie for a session that is already committed.
				if sess.ID() != initialID {
					if err := store.Drop(ctx, initialID); err != nil {
						logger.Error("failed to drop rotated session", "err", err)
					}
				}
			}

			ctx := WithSession(r.Context(), sess)

			next.ServeHTTP(rw, r.WithContext(ctx))

			// Handlers that never write leave the save hook untripped.
			rw.saveOnce()
		})
	}
}

// SessionFromContext retrieves the session from the provided context.
// The returned boolean indicates whether a session was present.
func SessionFromContext(ctx context.Context) (*session.Session, bool) {
	sess, ok := ctx.Value(ctxKeySession{}).(*session.Session)
	return sess, ok
}

// WithSession attaches the session to the context.
// Intended for use in [Session] middleware and tests only.
func WithSession(ctx context.Context, sess *session.Session) context.Context {
	return context.WithValue(ctx, ctxKeySession{}, sess)
}
