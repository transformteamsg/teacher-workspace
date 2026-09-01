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

// pgClaims carries the identity PG's /api/tw/1 surface requires: it resolves
// the staff member from sub scoped by school_code, then authorises on roles.
type pgClaims struct {
	Roles         []string `json:"roles"`
	EffectiveRole string   `json:"effective_role"`
	Attributes    []string `json:"attributes"`
	SchoolCode    string   `json:"school_code"`

	jwt.RegisteredClaims
}

// proxy forwards a request to that app's backend, stripping the
// /api/<app> prefix. It swaps the session cookie for JWT. Responds 404
// for unknown apps and 500 if the token cannot be signed.
func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())
	app := r.PathValue("app")

	now := time.Now()
	reg := jwt.RegisteredClaims{
		Issuer:    "TW",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(h.cfg.APIProxy.TokenTTL)),
	}

	var p *stdhttputil.ReverseProxy
	var signingKey string
	var claims jwt.Claims
	switch app {
	case "student-insights":
		p, signingKey = h.studentInsightsProxy, h.cfg.APIProxy.StudentInsightsSigningKey
		reg.Audience = jwt.ClaimStrings{"si"}
		claims = reg
	case "posts":
		p, signingKey = h.postsProxy, h.cfg.APIProxy.PostsSigningKey
		identity, err := pgIdentityFor(r, logger, h.cfg.Env)
		if err != nil {
			logger.Error("refusing to sign a PG token", "err", err)
			httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
				Message: http.StatusText(http.StatusInternalServerError),
			})
			return
		}
		reg.Audience = jwt.ClaimStrings{"pg"}
		reg.Subject = identity.Subject
		claims = pgClaims{
			Roles:            identity.Roles,
			EffectiveRole:    identity.EffectiveRole,
			Attributes:       identity.Attributes,
			SchoolCode:       identity.SchoolCode,
			RegisteredClaims: reg,
		}
	default:
		httputil.RenderJSON(w, logger, http.StatusNotFound, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusNotFound),
		})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(signingKey))
	if err != nil {
		logger.Error("failed to sign JWT", "app", app, "err", err)
		httputil.RenderJSON(w, logger, http.StatusInternalServerError, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusInternalServerError),
		})
		return
	}

	r.Header.Del("Cookie")
	r.Header.Set("Authorization", "Bearer "+signedToken)

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
