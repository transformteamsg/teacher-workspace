package handler

import (
	"net/http"
	"net/http/httputil"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
)

// Handler represents a handler for the application.
type Handler struct {
	cfg *config.Config

	proxy                *httputil.ReverseProxy
	studentInsightsProxy *httputil.ReverseProxy
	postsProxy           *httputil.ReverseProxy
	assets               http.Handler
}

// New creates a new Handler.
func New(cfg *config.Config) *Handler {
	h := &Handler{
		cfg: cfg,
		studentInsightsProxy: &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(cfg.APIProxy.StudentInsightsBaseURL)
			},
			ErrorHandler: handleProxyError,
		},
		postsProxy: &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(cfg.APIProxy.PostsBaseURL)
			},
			ErrorHandler: handleProxyError,
		},
	}

	switch cfg.Env {
	case config.EnvDevelopment:
		h.proxy = httputil.NewSingleHostReverseProxy(cfg.DevServerURL)
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
	app.HandleFunc("/api/", handleProxyNotFound)
	app.HandleFunc("/api/student-insights/", h.studentInsights)
	app.HandleFunc("/api/posts/", h.posts)

	mux.Handle("/", session(app))
}
