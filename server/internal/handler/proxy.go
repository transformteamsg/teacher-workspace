package handler

import (
	"context"
	"errors"
	"mime"
	"net/http"
	stdhttputil "net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
)

type ctxKeySignedToken struct{}

type remoteBackend struct {
	proxy      http.Handler
	signingKey string
	audience   string
}

func (h *Handler) proxy() http.Handler {
	remoteBackends := map[string]remoteBackend{}

	if h.cfg.RemoteApps.IsPostsRegistered() {
		remoteBackends["posts"] = remoteBackend{
			proxy: newRemoteBackendProxy(
				h.cfg.RemoteApps.PostsBackendBaseURL,
				h.cfg.RemoteApps.ProxyTimeout,
				h.cfg.RemoteApps.ProxyStreamTimeout,
			),
			signingKey: h.cfg.RemoteApps.PostsBackendSigningKey,
			audience:   "pg",
		}
	}
	if h.cfg.RemoteApps.IsStudentInsightsRegistered() {
		remoteBackends["student-insights"] = remoteBackend{
			proxy: newRemoteBackendProxy(
				h.cfg.RemoteApps.StudentInsightsBackendBaseURL,
				h.cfg.RemoteApps.ProxyTimeout,
				h.cfg.RemoteApps.ProxyStreamTimeout,
			),
			signingKey: h.cfg.RemoteApps.StudentInsightsBackendSigningKey,
			audience:   "si",
		}
	}

	now := h.now
	if now == nil {
		now = time.Now
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.LoggerFromContext(r.Context())

		name, _, found := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
		b, ok := remoteBackends[name]
		if !found || !ok {
			httputil.RenderJSON(w, logger, http.StatusNotFound, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusNotFound),
			})
			return
		}

		sess, ok := middleware.SessionFromContext(r.Context())
		if !ok {
			logger.Error("no session found in context")
			httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusInternalServerError),
			})
			return
		}

		u, ok := sess.User()
		if !ok {
			logger.Error("no user found in session")
			httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusInternalServerError),
			})
			return
		}
		if u.ID == "" || u.Email == "" || u.Role == "" || u.SchoolCode == "" || u.Attributes == nil {
			logger.Error("incomplete user found in session")
			httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusInternalServerError),
			})
			return
		}

		issuedAt := now()
		claims := struct {
			jwt.RegisteredClaims

			Email string `json:"email"`
			Role  string `json:"role"`
			// EffectiveRole duplicates Role for PG. Remove it once PG reads role.
			EffectiveRole string   `json:"effective_role"`
			SchoolCode    string   `json:"school_code"`
			Attributes    []string `json:"attributes"`
		}{
			Issuer:    "tw",
			Subject:   u.ID,
			Audience:  jwt.ClaimStrings{b.audience},
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(h.cfg.RemoteApps.SignedTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(issuedAt),

			Email:         u.Email,
			Role:          u.Role,
			EffectiveRole: u.Role,
			SchoolCode:    u.SchoolCode,
			Attributes:    u.Attributes,
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedToken, err := token.SignedString([]byte(b.signingKey))
		if err != nil {
			logger.Error("failed to sign JWT", "app", name, "err", err)
			httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusInternalServerError),
			})
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeySignedToken{}, signedToken)
		http.StripPrefix("/api/"+name, b.proxy).ServeHTTP(w, r.WithContext(ctx))
	})
}

// newRemoteBackendProxy returns a handler that proxies requests to target. A
// request is cut off once timeout has passed, with a 504 if the remote backend
// has not sent its response headers by then. An event stream with a 200 status
// or an upgraded connection, such as a WebSocket, instead stays open until
// streamTimeout has passed since its response headers arrived, even past the
// server's write timeout.
func newRemoteBackendProxy(target *url.URL, timeout, streamTimeout time.Duration) http.Handler {
	base := &stdhttputil.ReverseProxy{
		Rewrite: func(pr *stdhttputil.ProxyRequest) {
			pr.SetURL(target)

			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("X-CSRF-Token")
			if signedToken, ok := pr.In.Context().Value(ctxKeySignedToken{}).(string); ok {
				pr.Out.Header.Set("Authorization", "Bearer "+signedToken)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if cause := context.Cause(r.Context()); cause != nil {
				err = cause
			}
			if errors.Is(err, context.Canceled) {
				// Cancellation means the client disconnected, which is expected.
				// Logging it would only add noise, and the response would be lost.
				return
			}

			path, _, _ := strings.Cut(r.RequestURI, "?")
			remoteBackendURL := *r.URL
			remoteBackendURL.RawQuery = ""

			logger := middleware.LoggerFromContext(r.Context())
			logger.Error("failed to proxy request",
				"method", r.Method,
				"path", path,
				"remote_backend_url", remoteBackendURL.String(),
				"err", err,
			)

			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				// A deadline means the remote backend did not respond within
				// the proxy's timeouts.
				status = http.StatusGatewayTimeout
			}

			httputil.RenderJSON(w, logger, status, &httputil.ErrorResponse{
				Message: http.StatusText(status),
			})
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancelCause(r.Context())
		defer cancel(nil)

		timer := time.AfterFunc(timeout, func() {
			cancel(context.DeadlineExceeded)
		})
		defer timer.Stop()

		proxy := *base
		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")

			mediaType, _, _ := mime.ParseMediaType(resp.Header.Get(httputil.HeaderContentType))
			switch {
			case resp.StatusCode == http.StatusSwitchingProtocols:
				// The connection is switching protocols, such as to a
				// WebSocket. The proxy hijacks it, which clears its write
				// deadline, so only the timer needs extending.
			case resp.StatusCode == http.StatusOK && mediaType == httputil.MIMETextEventStream:
				// Keep the write deadline past the stream timeout, so the
				// stream is closed by the timeout rather than a failed write.
				deadline := time.Now().Add(streamTimeout + 5*time.Second)
				if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
					return err
				}
			default:
				return nil
			}

			// Don't revive a request whose timeout has already fired.
			if !timer.Stop() {
				return context.DeadlineExceeded
			}
			timer.Reset(streamTimeout)
			return nil
		}

		proxy.ServeHTTP(w, r.WithContext(ctx))
	})
}
