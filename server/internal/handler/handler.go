package handler

import (
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strings"

	"github.com/String-sg/teacher-workspace/server/internal/config"
)

// Handler serves the apps/host frontend: proxied from the rsbuild dev server
// in development, or served from its build output directory in production.
type Handler struct {
	cfg *config.Config

	proxy  *httputil.ReverseProxy
	assets http.Handler
}

// New builds a Handler for the given config.
func New(cfg *config.Config) *Handler {
	h := &Handler{cfg: cfg}

	switch cfg.Env {
	case config.EnvDevelopment:
		h.proxy = httputil.NewSingleHostReverseProxy(cfg.Host.DevServerURL)
	case config.EnvProduction:
		h.assets = http.FileServer(http.Dir(cfg.Host.BuildDir))
	}

	return h
}

// NewMux returns a ServeMux with all application routes registered.
func NewMux(cfg *config.Config) *http.ServeMux {
	h := New(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.ServeHost)
	return mux
}

// ServeHost serves the host app for any GET request: proxying to the rsbuild
// dev server in development, or serving the build output (falling back to
// index.html for unknown non-asset routes so client-side routing works) in
// production.
func (h *Handler) ServeHost(w http.ResponseWriter, r *http.Request) {
	switch h.cfg.Env {
	case config.EnvDevelopment:
		h.proxy.ServeHTTP(w, r)
	case config.EnvProduction:
		if strings.HasPrefix(r.URL.Path, "/static/") {
			h.assets.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(h.cfg.Host.BuildDir, "index.html"))
	}
}
