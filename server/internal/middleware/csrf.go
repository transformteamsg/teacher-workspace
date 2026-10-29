package middleware

import (
	"net/http"

	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
)

// CSRFTokenHeader is the request header that carries a CSRF token.
const CSRFTokenHeader = "X-CSRF-Token"

// CSRF returns a middleware that serves a GET, HEAD, OPTIONS, or QUERY request
// with the handler it wraps. It serves any other request with that handler only
// if the request is same-origin and its X-CSRF-Token header holds a token that
// verifies against the request's session, and with reject otherwise. Each
// rejection is logged as a warning.
//
// A request is same-origin when its Sec-Fetch-Site header is same-origin or
// none. Without Sec-Fetch-Site, it is same-origin when its Origin header is
// missing or names the request's host.
//
// The middleware must run inside [Session], which attaches the session it
// reads. If the request has no session, it responds with 500 and does not
// serve either handler.
func CSRF(reject http.HandlerFunc) Middleware {
	origin := http.NewCrossOriginProtection()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// QUERY is safe, like GET, but CrossOriginProtection checks it as an
			// unsafe method, and net/http has no constant for it.
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, "QUERY":
				next.ServeHTTP(w, r)
				return
			}

			logger := LoggerFromContext(r.Context())

			sess, ok := SessionFromContext(r.Context())
			if !ok {
				logger.Error("no session found in context")
				httputil.RenderPlain(w, logger, http.StatusInternalServerError)
				return
			}

			if err := origin.Check(r); err != nil {
				logger.Warn("rejected cross-origin request", "err", err)
				reject(w, r)
				return
			}

			if !sess.VerifyCSRFToken(r.Header.Get(CSRFTokenHeader)) {
				logger.Warn("rejected request without a valid CSRF token")
				reject(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
