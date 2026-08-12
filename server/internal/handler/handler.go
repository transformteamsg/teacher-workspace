package handler

import (
	"net/http"
	stdhttputil "net/http/httputil"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// Handler represents a handler for the application.
type Handler struct {
	cfg *config.Config

	devProxy             *stdhttputil.ReverseProxy
	studentInsightsProxy *stdhttputil.ReverseProxy
	postsProxy           *stdhttputil.ReverseProxy
	assets               http.Handler
}

// New creates a new Handler.
func New(cfg *config.Config) *Handler {
	h := &Handler{
		cfg: cfg,
		studentInsightsProxy: &stdhttputil.ReverseProxy{
			Rewrite: func(pr *stdhttputil.ProxyRequest) {
				pr.SetURL(cfg.APIProxy.StudentInsightsBaseURL)
			},
			ErrorHandler: proxyErrorHandler,
		},
		postsProxy: &stdhttputil.ReverseProxy{
			Rewrite: func(pr *stdhttputil.ProxyRequest) {
				pr.SetURL(cfg.APIProxy.PostsBaseURL)
			},
			ErrorHandler: proxyErrorHandler,
		},
	}

	switch cfg.Env {
	case config.EnvDevelopment:
		h.devProxy = stdhttputil.NewSingleHostReverseProxy(cfg.DevServerURL)
	case config.EnvProduction:
		h.assets = http.FileServer(http.Dir(cfg.BuildDir))
	}

	return h
}

// Register registers all application routes on the given HTTP server mux.
// Application routes are wrapped in the session middleware; static asset routes
// are not.
func (h *Handler) Register(mux *http.ServeMux, session middleware.Middleware) {
	mux.HandleFunc("/static/", h.static)

	// Session-scoped routes: everything registered on this sub-mux runs
	// through the session middleware, which is applied a single time.
	app := http.NewServeMux()
	app.HandleFunc("/", h.index)

	app.HandleFunc("/api/{app}/", h.proxy)
	app.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.LoggerFromContext(r.Context())
		httputil.RenderJSON(w, logger, http.StatusNotFound, &httputil.ErrorResponse{
			Message: http.StatusText(http.StatusNotFound),
		})
	})

	mux.Handle("/", session(app))
}
