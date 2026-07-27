package handler

import (
	"net/http"
	"net/http/httputil"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// Handler represents a handler for the application.
type Handler struct {
	cfg *config.Config

	proxy  *httputil.ReverseProxy
	assets http.Handler
}

// New creates a new Handler.
func New(cfg *config.Config) *Handler {
	h := &Handler{cfg: cfg}

	switch cfg.Env {
	case config.EnvDevelopment:
		h.proxy = httputil.NewSingleHostReverseProxy(cfg.DevServerURL)
	case config.EnvProduction:
		h.assets = http.FileServer(http.Dir(cfg.BuildDir))
	}

	return h
}

// Register registers all application routes on the given HTTP server mux. The
// session middleware is applied to the application routes but not to static
// assets, so asset responses carry no session cookie and skip the session store.
func (h *Handler) Register(mux *http.ServeMux, session middleware.Middleware) {
	// Static assets bypass the session middleware entirely.
	mux.HandleFunc("/static/", h.static)

	// Session-scoped routes: everything registered on this sub-mux runs
	// through the session middleware, which is applied a single time.
	app := http.NewServeMux()
	app.HandleFunc("/", h.index)

	mux.Handle("/", session(app))
}
