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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.LoggerFromContext(r.Context())

		sess, ok := middleware.SessionFromContext(r.Context())
		if !ok {
			logger.Error("no session found in context")
			httputil.RenderPlain(w, logger, http.StatusInternalServerError)
			return
		}

		state := &PreloadedState{
			CSRFToken: sess.CSRFToken(),
			Remotes:   []Remote{},
			Apps:      []string{},
		}
		if user, ok := sess.User(); ok {
			// A nil list is encoded as null, which the page rejects.
			attributes := user.Attributes
			if attributes == nil {
				attributes = []string{}
			}

			state.User = &User{
				Name:       user.Name,
				Email:      user.Email,
				Role:       edupassRoleLabel(user.Role),
				RoleCode:   user.Role,
				Attributes: attributes,
			}
			if name, ok := h.schoolNames[user.SchoolCode]; ok {
				state.User.School = name
			}
			state.Remotes, state.Apps = h.allowedApps(user.SchoolCode)
		}

		h.renderIndexTemplate(w, r, state)
	})
}

// allowedApps returns the remotes and app keys the page offers the school with schoolCode.
// It offers each app that is registered and whose allowlist allows the school.
func (h *Handler) allowedApps(schoolCode string) ([]Remote, []string) {
	remotes := []Remote{}
	apps := []string{}

	for _, a := range h.cfg.RemoteApps.RemoteApps() {
		if a.IsRegistered() && a.Allowlist.Allows(schoolCode) {
			remotes = append(remotes, Remote{Name: a.RemoteName, Entry: a.ManifestURL.String()})
			apps = append(apps, a.App)
		}
	}

	return remotes, apps
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
