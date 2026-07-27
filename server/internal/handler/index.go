package handler

import (
	"net/http"
	"path/filepath"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// index serves the frontend's application shell. In development it proxies to
// the rsbuild dev server; in production it serves index.html for all routes so
// client-side routing works.
func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	switch h.cfg.Env {
	case config.EnvDevelopment:
		h.proxy.ServeHTTP(w, r)
	case config.EnvProduction:
		http.ServeFile(w, r, filepath.Join(h.cfg.BuildDir, "index.html"))
	default:
		logger := middleware.LoggerFromContext(r.Context())
		httputil.RenderPlain(w, logger, http.StatusNotFound)
	}
}

// static serves the frontend's hashed static assets. In development it proxies
// to the rsbuild dev server; in production it serves files from the build
// directory.
func (h *Handler) static(w http.ResponseWriter, r *http.Request) {
	switch h.cfg.Env {
	case config.EnvDevelopment:
		h.proxy.ServeHTTP(w, r)
	case config.EnvProduction:
		h.assets.ServeHTTP(w, r)
	default:
		logger := middleware.LoggerFromContext(r.Context())
		httputil.RenderPlain(w, logger, http.StatusNotFound)
	}
}
