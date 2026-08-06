package handler

import (
	"net/http"
	"strings"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// Forwards /api/posts/<path> to the posts backend.
func (h *Handler) posts(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/api/posts", h.postsProxy).ServeHTTP(w, r)
}

// Forwards /api/student-insights/<path> to the student-insights backend.
func (h *Handler) studentInsights(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/api/student-insights", h.studentInsightsProxy).ServeHTTP(w, r)
}

// Logs the failure when the backend fails.
func handleProxyError(w http.ResponseWriter, r *http.Request, err error) {
	path, _, _ := strings.Cut(r.RequestURI, "?")
	backend := *r.URL
	backend.RawQuery = ""

	middleware.LoggerFromContext(r.Context()).Error("failed to proxy request",
		"method", r.Method,
		"path", path,
		"backend", backend,
		"err", err,
	)

	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

// Answers 404 for an /api/ path with no proxy behind it.
func handleProxyNotFound(w http.ResponseWriter, r *http.Request) {
	httputil.RenderPlain(w, middleware.LoggerFromContext(r.Context()), http.StatusNotFound)
}
