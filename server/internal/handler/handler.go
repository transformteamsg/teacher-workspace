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

// New returns an http.Handler with all application routes registered.
func New(cfg *config.Config) http.Handler {
	h := &Handler{cfg: cfg}

	switch cfg.Env {
	case config.EnvDevelopment:
		h.proxy = httputil.NewSingleHostReverseProxy(cfg.DevServerURL)
	case config.EnvProduction:
		h.assets = http.FileServer(http.Dir(cfg.BuildDir))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.ServeHost)
	return mux
}

// ServeHost serves the host app for any request: proxying to the rsbuild dev
// server in development, or serving the build output (falling back to
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
		http.ServeFile(w, r, filepath.Join(h.cfg.BuildDir, "index.html"))
	}
}
