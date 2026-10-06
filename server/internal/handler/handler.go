package handler

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	stdhttputil "net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/htmlutil"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var errDevServerResourceNotFound = errors.New("resource not found")

type Handler struct {
	cfg *config.Config

	indexTemplate  htmlutil.Template
	devServerProxy http.Handler
	prdFileSystem  fs.FS

	edupassHTTPClient      *http.Client
	edupassOAuth2Config    *oauth2.Config
	edupassIDTokenVerifier *oidc.IDTokenVerifier

	// now is the time source for signed tokens. Nil means [time.Now].
	now func() time.Time
}

func New(cfg *config.Config) (*Handler, error) {
	edupassHTTPClient := &http.Client{Timeout: 10 * time.Second}

	h := &Handler{
		cfg:               cfg,
		edupassHTTPClient: edupassHTTPClient,
		edupassOAuth2Config: &oauth2.Config{
			ClientID:     cfg.Edupass.ClientID,
			ClientSecret: cfg.Edupass.ClientCredentials.Secret,
			RedirectURL:  cfg.Edupass.RedirectURL.String(),
			Endpoint: oauth2.Endpoint{
				AuthURL:   cfg.Edupass.AuthURL.String(),
				TokenURL:  cfg.Edupass.TokenURL.String(),
				AuthStyle: oauth2.AuthStyleInParams,
			},
			Scopes: []string{oidc.ScopeOpenID},
		},
		edupassIDTokenVerifier: oidc.NewVerifier(
			cfg.Edupass.IssuerURL.String(),
			oidc.NewRemoteKeySet(
				oidc.ClientContext(context.Background(), edupassHTTPClient),
				cfg.Edupass.JWKSURL.String(),
			),
			&oidc.Config{ClientID: cfg.Edupass.ClientID},
		),
	}

	switch cfg.Env {
	case config.EnvDevelopment:
		h.indexTemplate = htmlutil.NewURLTemplate(cfg.DevServerURL.String())

		h.devServerProxy = newDevServerProxy(cfg.DevServerURL)
	case config.EnvProduction:
		tmpl, err := htmlutil.NewFileTemplate(filepath.Join(cfg.BuildDir, "index.html"))
		if err != nil {
			return nil, fmt.Errorf("load index template: %w", err)
		}
		h.indexTemplate = tmpl

		root, err := os.OpenRoot(cfg.BuildDir)
		if err != nil {
			return nil, fmt.Errorf("open build directory: %w", err)
		}
		h.prdFileSystem = root.FS()
	default:
		return nil, fmt.Errorf("unsupported environment: %s", cfg.Env)
	}

	return h, nil
}

// Routes returns the application's routes. Every route except static assets
// runs through session, which must be the middleware returned by
// [middleware.Session].
func (h *Handler) Routes(session middleware.Middleware) http.Handler {
	app := http.NewServeMux()
	app.HandleFunc("GET /auth/edupass", h.authEdupass)
	app.HandleFunc("GET /auth/edupass/callback", h.authEdupassCallback)
	app.HandleFunc("/api/", h.proxy())
	app.HandleFunc("/", h.index())

	mux := http.NewServeMux()
	mux.HandleFunc("/static/", h.static)
	mux.Handle("/", session(app))

	return mux
}

func newDevServerProxy(target *url.URL) *stdhttputil.ReverseProxy {
	return &stdhttputil.ReverseProxy{
		Rewrite: func(pr *stdhttputil.ProxyRequest) {
			pr.SetURL(target)
		},
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode == http.StatusNotFound {
				return errDevServerResourceNotFound
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger := middleware.LoggerFromContext(r.Context())

			if errors.Is(err, errDevServerResourceNotFound) {
				httputil.RenderPlain(w, logger, http.StatusNotFound)
				return
			}

			logger.Error("failed to proxy request to dev server", "err", err)
			httputil.RenderPlain(w, logger, http.StatusBadGateway)
		},
	}
}
