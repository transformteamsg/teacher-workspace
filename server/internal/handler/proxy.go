package handler

import (
	"context"
	"fmt"
	"net/http"
	stdhttputil "net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
)

// ctxKeyJWT keys the token minted for the request being proxied.
type ctxKeyJWT struct{}

// proxy sends an MFE's API request to that app's backend, stripping the
// /api/<app> prefix. It swaps the user's session cookie for a short-lived
// signed token. Answers 404 for an unknown app and 500 if the token cannot
// be signed.
func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())
	app := r.PathValue("app")

	var p *stdhttputil.ReverseProxy
	var aud, key string
	switch app {
	case "posts":
		p, aud, key = h.postsProxy, "pg", h.cfg.APIProxy.PostsSigningKey
	case "student-insights":
		p, aud, key = h.studentInsightsProxy, "si", h.cfg.APIProxy.StudentInsightsSigningKey
	default:
		httputil.RenderJSON(w, logger, http.StatusNotFound, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusNotFound),
		})
		return
	}

	token, err := signToken(aud, key, h.cfg.APIProxy.TokenTTL)
	if err != nil {
		logger.Error("failed to sign JWT", "app", app, "err", err)
		httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusInternalServerError),
		})

		return
	}

	r = r.WithContext(context.WithValue(r.Context(), ctxKeyJWT{}, token))

	http.StripPrefix("/api/"+app, p).ServeHTTP(w, r)
}

// proxyRewriter returns a Rewrite hook that sends the request to target. It
// removes the session cookie from the outbound request and replaces it with a
// signed JWT.
func proxyRewriter(target *url.URL) func(*stdhttputil.ProxyRequest) {
	return func(pr *stdhttputil.ProxyRequest) {
		pr.SetURL(target)

		pr.Out.Header.Del("Cookie")
		if token, ok := pr.In.Context().Value(ctxKeyJWT{}).(string); ok {
			pr.Out.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

// Logs the failure and answers 502 when a backend request fails. Query strings
// are dropped to keep credentials and other sensitive values out of the logs.
func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	path, _, _ := strings.Cut(r.RequestURI, "?")
	backend := *r.URL
	backend.RawQuery = ""

	logger := middleware.LoggerFromContext(r.Context())
	logger.Error("failed to proxy request",
		"method", r.Method,
		"path", path,
		"backend", backend.String(),
		"err", err,
	)

	httputil.RenderJSON(w, logger, http.StatusBadGateway, &httputil.ErrorResponse{
		Message: http.StatusText(http.StatusBadGateway),
	})
}

// signToken returns a signed JWT that identifies TW to the downstream backends.
func signToken(audience, key string, ttl time.Duration) (string, error) {
	now := time.Now()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "TW",
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	})

	signedJWT, err := token.SignedString([]byte(key))
	if err != nil {
		return "", fmt.Errorf("sign token for %q: %w", audience, err)
	}

	return signedJWT, nil
}
