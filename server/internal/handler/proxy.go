package handler

import (
	"context"
	"errors"
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
			proxy:      newRemoteBackendProxy(h.cfg.RemoteApps.PostsBackendBaseURL),
			signingKey: h.cfg.RemoteApps.PostsBackendSigningKey,
			audience:   "pg",
		}
	}
	if h.cfg.RemoteApps.IsStudentInsightsRegistered() {
		remoteBackends["student-insights"] = remoteBackend{
			proxy:      newRemoteBackendProxy(h.cfg.RemoteApps.StudentInsightsBackendBaseURL),
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

		// A user without an Edupass ID, role or attributes has no claims that a
		// remote app backend could act on.
		var missing []string
		if u.ID == "" {
			missing = append(missing, "edupass_id")
		}
		if u.Role == "" {
			missing = append(missing, "role")
		}
		if u.Attributes == nil {
			missing = append(missing, "attributes")
		}
		if len(missing) > 0 {
			logger.Error("missing claims found in user", "missing", missing)
			httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusInternalServerError),
			})
			return
		}

		issuedAt := now()
		claims := struct {
			jwt.RegisteredClaims

			Role string `json:"role"`
			// EffectiveRole duplicates Role for PG. Remove it once PG reads role.
			EffectiveRole string   `json:"effective_role"`
			Attributes    []string `json:"attributes"`
		}{
			Issuer:    "tw",
			Subject:   u.ID,
			Audience:  jwt.ClaimStrings{b.audience},
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(h.cfg.RemoteApps.SignedTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(issuedAt),

			Role:          u.Role,
			EffectiveRole: u.Role,
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

func newRemoteBackendProxy(target *url.URL) *stdhttputil.ReverseProxy {
	return &stdhttputil.ReverseProxy{
		Rewrite: func(pr *stdhttputil.ProxyRequest) {
			pr.SetURL(target)

			pr.Out.Header.Del("Cookie")
			if signedToken, ok := pr.In.Context().Value(ctxKeySignedToken{}).(string); ok {
				pr.Out.Header.Set("Authorization", "Bearer "+signedToken)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
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

			httputil.RenderJSON(w, logger, http.StatusBadGateway, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusBadGateway),
			})
		},
	}
}
