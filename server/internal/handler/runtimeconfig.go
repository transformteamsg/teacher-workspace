package handler

import (
	"net/http"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// runtimeConfigResponse is the document served at /config.json.
type runtimeConfigResponse struct {
	Remotes []config.Remote `json:"remotes"`
}

// runtimeConfig serves the remotes the frontend registers at startup. It
// carries no user data, so it is registered outside the session middleware.
func (h *Handler) runtimeConfig(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	// An empty array rather than null, so the client never has to guard
	// against a missing list.
	remotes := h.cfg.Host.ParsedRemotes()
	if remotes == nil {
		remotes = []config.Remote{}
	}

	// A cached copy would keep pointing the host at a previous run's remote.
	w.Header().Set("Cache-Control", "no-store")

	httputil.RenderJSON(w, logger, http.StatusOK, &runtimeConfigResponse{Remotes: remotes})
}
