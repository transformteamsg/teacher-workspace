package handler

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
)

func (h *Handler) index() http.Handler {
	remotes := []Remote{}

	if h.cfg.RemoteApps.IsPostsRegistered() {
		remotes = append(remotes, Remote{Name: "pg", Entry: h.cfg.RemoteApps.PostsManifestURL.String()})
	}
	if h.cfg.RemoteApps.IsStudentInsightsRegistered() {
		remotes = append(remotes, Remote{Name: "si", Entry: h.cfg.RemoteApps.StudentInsightsManifestURL.String()})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.LoggerFromContext(r.Context())

		sess, ok := middleware.SessionFromContext(r.Context())
		if !ok {
			logger.Error("no session found in context")
			httputil.RenderPlain(w, logger, http.StatusInternalServerError)
			return
		}

		h.renderIndexTemplate(w, r, &PreloadedState{
			CSRFToken: sess.CSRFToken(),
			Remotes:   remotes,
		})
	})
}

func (h *Handler) static(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	switch h.cfg.Env {
	case config.EnvDevelopment:
		h.devServerProxy.ServeHTTP(w, r)
	case config.EnvProduction:
		name := strings.TrimPrefix(r.URL.Path, "/")
		info, err := fs.Stat(h.prdFileSystem, name)
		if err != nil || info.IsDir() {
			httputil.RenderPlain(w, logger, http.StatusNotFound)
			return
		}

		http.ServeFileFS(w, r, h.prdFileSystem, name)
	default:
		httputil.RenderPlain(w, logger, http.StatusNotFound)
	}
}
