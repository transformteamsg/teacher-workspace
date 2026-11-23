package middleware

import (
	"net/http"

	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
)

// RequireAuth returns a middleware that serves a request with the handler it
// wraps only if the request's session is authenticated, and with fallback
// otherwise.
//
// The middleware must run inside [Session], which attaches the session it
// reads. If the request has no session, it responds with 500 and serves
// neither handler.
func RequireAuth(fallback http.HandlerFunc) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				logger := LoggerFromContext(r.Context())
				logger.Error("no session found in context")
				httputil.RenderPlain(w, logger, http.StatusInternalServerError)
				return
			}

			if !sess.IsAuthenticated() {
				fallback(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
