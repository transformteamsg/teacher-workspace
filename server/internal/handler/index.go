package handler

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"

	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
)

type PreloadedState struct {
	CSRFToken string   `json:"csrfToken"`
	Remotes   []Remote `json:"remotes"`
	User      *User    `json:"user,omitempty"`
}

type Remote struct {
	Name  string `json:"name"`
	Entry string `json:"entry"`
}

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	// Role is the resolved role's display label, such as "Teacher", not its
	// Edupass code.
	Role string `json:"role"`
}

func (h *Handler) index() http.HandlerFunc {
	remotes := []Remote{}

	if h.cfg.RemoteApps.IsPostsRegistered() {
		remotes = append(remotes, Remote{Name: "pg", Entry: h.cfg.RemoteApps.PostsManifestURL.String()})
	}
	if h.cfg.RemoteApps.IsStudentInsightsRegistered() {
		remotes = append(remotes, Remote{Name: "si", Entry: h.cfg.RemoteApps.StudentInsightsManifestURL.String()})
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.LoggerFromContext(r.Context())
		sess, ok := middleware.SessionFromContext(r.Context())
		if !ok {
			logger.Error("no session found in context")
			httputil.RenderPlain(w, logger, http.StatusInternalServerError)
			return
		}

		preloadedState := &PreloadedState{
			CSRFToken: sess.CSRFToken(),
			Remotes:   remotes,
		}
		if user, ok := sess.User(); ok {
			preloadedState.User = &User{
				Name:  user.Name,
				Email: user.Email,
				Role:  edupassRoleLabel(user.Role),
			}
		}

		var buf bytes.Buffer
		if err := h.indexTemplate.Execute(r.Context(), &buf, preloadedState); err != nil {
			logger.Error("failed to execute index template", "err", err)
			httputil.RenderPlain(w, logger, http.StatusInternalServerError)
			return
		}

		httputil.RenderHTML(w, logger, http.StatusOK, buf.Bytes())
	}
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
