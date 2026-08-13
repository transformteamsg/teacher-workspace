package handler

import (
	"net/http"
	stdhttputil "net/http/httputil"
	"strings"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
)

// proxy forwards a request to that app's backend, stripping the
// /api/<app> prefix. It swaps the session cookie for JWT. Responds 404
// for unknown apps and 500 if the token cannot be signed.
func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())
	app := r.PathValue("app")

	var p *stdhttputil.ReverseProxy
	var signingKey, aud string
	switch app {
	case "student-insights":
		p, signingKey, aud = h.studentInsightsProxy, h.cfg.APIProxy.StudentInsightsSigningKey, "si"
	case "posts":
		p, signingKey, aud = h.postsProxy, h.cfg.APIProxy.PostsSigningKey, "pg"
	default:
		httputil.RenderJSON(w, logger, http.StatusNotFound, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusNotFound),
		})
		return
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "TW",
		Audience:  jwt.ClaimStrings{aud},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(h.cfg.APIProxy.TokenTTL)),
	})
	signedJWT, err := token.SignedString([]byte(signingKey))
	if err != nil {
		logger.Error("failed to sign JWT", "app", app, "err", err)
		httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusInternalServerError),
		})
		return
	}

	r.Header.Del("Cookie")
	r.Header.Set("Authorization", "Bearer "+signedJWT)

	http.StripPrefix("/api/"+app, p).ServeHTTP(w, r)
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
