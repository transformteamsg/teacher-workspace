package handler

import (
	"bytes"
	"net/http"
	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// runtimeRemote is a Module Federation remote the frontend registers at startup.
type runtimeRemote struct {
	Name  string `json:"name"`
	Entry string `json:"entry"`
}

// runtimeConfig is the document embedded in index.html for the frontend to read at startup.
type runtimeConfig struct {
	Remotes []runtimeRemote `json:"remotes"`
}

// newRuntimeConfig maps the configured manifest URLs to the remote names the
// frontend loads modules from, leaving out the remotes that are not set.
func newRuntimeConfig(cfg config.RemoteConfig) runtimeConfig {
	// An empty array rather than null, so the client never has to guard
	// against a missing list.
	remotes := []runtimeRemote{}

	// The pg remote exposes both the Posts and the Groups module.
	if cfg.PostsManifestURL != "" {
		remotes = append(remotes, runtimeRemote{Name: "pg", Entry: cfg.PostsManifestURL})
	}
	if cfg.StudentInsightsManifestURL != "" {
		remotes = append(remotes, runtimeRemote{Name: "si", Entry: cfg.StudentInsightsManifestURL})
	}

	return runtimeConfig{Remotes: remotes}
}

// index serves the frontend's application shell with the runtime config
// embedded. In development it templates the page fetched from the rsbuild dev
// server; in production it renders the template parsed at startup. Both modes
// serve all routes so client-side routing works.
func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	// Rendered into a buffer, so a failure part-way through never leaves the
	// browser with half a page and a success status.
	var page bytes.Buffer
	if err := h.executor.Execute(r.Context(), &page, h.runtime); err != nil {
		logger.Error("failed to render index.html", "err", err)

		// In development the page comes from the dev server, so a failure to
		// render it is a failure of that server rather than of this one.
		status := http.StatusInternalServerError
		if h.cfg.Env == config.EnvDevelopment {
			status = http.StatusBadGateway
		}
		httputil.RenderPlain(w, logger, status)
		return
	}

	// A cached page would keep the remote list of a previous run.
	w.Header().Set("Cache-Control", "no-store")
	httputil.RenderHTML(w, logger, http.StatusOK, page.Bytes())
}

// static serves the frontend's hashed static assets. In development it proxies
// to the rsbuild dev server; in production it serves files from the build
// directory.
func (h *Handler) static(w http.ResponseWriter, r *http.Request) {
	switch h.cfg.Env {
	case config.EnvDevelopment:
		h.devProxy.ServeHTTP(w, r)
	case config.EnvProduction:
		h.assets.ServeHTTP(w, r)
	default:
		logger := middleware.LoggerFromContext(r.Context())
		httputil.RenderPlain(w, logger, http.StatusNotFound)
	}
}
