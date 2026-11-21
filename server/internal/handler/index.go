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
// The app keys must match AppKey in apps/host/src/stores/preloaded-state.ts, or the host hides the app.
func (h *Handler) allowedApps(schoolCode string) ([]Remote, []string) {
	cfg := h.cfg.RemoteApps
	remotes := []Remote{}
	apps := []string{}

	if cfg.IsPostsRegistered() && cfg.PostsAllowedSchoolCodes.Allows(schoolCode) {
		remotes = append(remotes, Remote{Name: "pg", Entry: cfg.PostsManifestURL.String()})
		apps = append(apps, "posts")
	}
	// Groups shares the Posts manifest, which exposes both modules, so it
	// registers under the same remote name.
	if cfg.IsGroupsRegistered() && cfg.GroupsAllowedSchoolCodes.Allows(schoolCode) {
		remotes = append(remotes, Remote{Name: "pg", Entry: cfg.GroupsManifestURL.String()})
		apps = append(apps, "groups")
	}
	if cfg.IsStudentInsightsRegistered() && cfg.StudentsAllowedSchoolCodes.Allows(schoolCode) {
		remotes = append(remotes, Remote{Name: "student_insights", Entry: cfg.StudentInsightsManifestURL.String()})
		apps = append(apps, "students")
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
