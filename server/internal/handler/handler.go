package handler

import (
	"fmt"
	"io/fs"
	"net/http"
	stdhttputil "net/http/httputil"
	"path/filepath"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/htmlutil"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/oidc"
)

// Handler represents a handler for the application.
type Handler struct {
	cfg *config.Config

	rp *oidc.RelyingParty

	devProxy             *stdhttputil.ReverseProxy
	studentInsightsProxy *stdhttputil.ReverseProxy
	postsProxy           *stdhttputil.ReverseProxy
	assets               http.Handler
	executor             htmlutil.TemplateExecutor
	runtime              runtimeConfig
}

// New creates a new Handler. In production it parses index.html once, so a
// missing or malformed page fails here rather than on the first request.
func New(cfg *config.Config, rp *oidc.RelyingParty) (*Handler, error) {
	h := &Handler{
		cfg:     cfg,
		runtime: newRuntimeConfig(cfg.Remote),
		rp:      rp,
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
		h.executor = htmlutil.NewDevelopmentTemplateExecutor(cfg.DevServerURL.String())
	case config.EnvProduction:
		h.assets = http.FileServer(fileOnlyFS{http.Dir(cfg.BuildDir)})

		executor, err := htmlutil.NewProductionTemplateExecutor(filepath.Join(cfg.BuildDir, "index.html"))
		if err != nil {
			return nil, fmt.Errorf("index.html: %w", err)
		}
		h.executor = executor
	}

	return h, nil
}

type fileOnlyFS struct {
	http.FileSystem
}

func (fsys fileOnlyFS) Open(name string) (http.File, error) {
	f, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, fs.ErrNotExist
	}

	return f, nil
}

// Register registers all application routes on the given HTTP server mux.
// Application routes are wrapped in the session middleware; static asset routes
// are not.
func (h *Handler) Register(mux *http.ServeMux, session middleware.Middleware) {
	mux.HandleFunc("/static/", h.static)

	// Session-scoped routes: everything registered on this sub-mux runs
	// through the session middleware, which is applied a single time.
	app := http.NewServeMux()
	app.HandleFunc("GET /auth/edupass", h.authEdupass)
	app.HandleFunc("GET /auth/edupass/callback", h.authEdupassCallback)
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
