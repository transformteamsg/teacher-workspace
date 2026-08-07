package handler

import (
	"net/http"
	stdhttputil "net/http/httputil"
	"strings"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")

	var p *stdhttputil.ReverseProxy
	switch app {
	case "posts":
		p = h.postsProxy
	case "student-insights":
		p = h.studentInsightsProxy
	default:
		httputil.RenderPlain(w, middleware.LoggerFromContext(r.Context()), http.StatusNotFound)
		return
	}

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

	httputil.RenderPlain(w, logger, http.StatusBadGateway)
}
