package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

// RequireAuth returns a Middleware that enforces authentication on all routes
// except /login, /auth/edupass, and /auth/edupass/callback. Outcomes:
//
//  1. Authenticated request: passes through to the handler.
//  2. Unauthenticated browser request: redirect to /login with return_to.
//  3. Unauthenticated API request (/api/): 401 JSON.
//  4. Authenticated user at /login: redirect to /.
//  5. Auth or login route: passes through without a session check.
func RequireAuth() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			if path == "/auth/edupass" || path == "/auth/edupass/callback" {
				next.ServeHTTP(w, r)
				return
			}

			sess, ok := SessionFromContext(r.Context())

			if path == "/login" {
				if ok && sess.IsAuthenticated() {
					http.Redirect(w, r, "/", http.StatusFound)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if !ok || !sess.IsAuthenticated() {
				if strings.HasPrefix(path, "/api/") {
					logger := LoggerFromContext(r.Context())
					httputil.RenderJSON(w, logger, http.StatusUnauthorized, &httputil.ErrorResponse{
						Message: http.StatusText(http.StatusUnauthorized),
					})
					return
				}

				target := "/login"
				if uri := r.URL.RequestURI(); uri != "/" {
					target = "/login?return_to=" + url.QueryEscape(uri)
				}
				http.Redirect(w, r, target, http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
