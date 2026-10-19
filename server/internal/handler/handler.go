package handler

import (
	"bytes"
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

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/htmlutil"
	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
	"github.com/transformteamsg/teacher-workspace/server/internal/session"
	"golang.org/x/oauth2"
)

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
			ClientID:    cfg.Edupass.ClientID,
			RedirectURL: cfg.Edupass.RedirectURL.String(),
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

// Routes returns the application's routes, keeping sessions in store.
//
// These routes are open to anyone:
//   - static assets
//   - the login page
//   - the Edupass login routes
//
// Every other route requires a logged-in user. A request from a user who is
// not logged in:
//   - gets a 401 under /api/
//   - is otherwise redirected to the login page, carrying the requested URL to
//     return to
func (h *Handler) Routes(store session.Store) http.Handler {
	sessionOpts := middleware.SessionOptions{
		Name:             h.cfg.Session.Name,
		DefaultTTL:       h.cfg.Session.DefaultTTL,
		AuthenticatedTTL: h.cfg.Session.AuthenticatedTTL,
		Secure:           h.cfg.Session.Secure,
	}

	app := http.NewServeMux()
	app.HandleFunc("GET /login", h.login)
	app.HandleFunc("POST /logout", h.logout)
	app.HandleFunc("GET /auth/edupass", h.authEdupass)
	app.HandleFunc("GET /auth/edupass/callback", h.authEdupassCallback)
	app.Handle("/api/", middleware.RequireAuth(renderUnauthorized)(h.proxy()))
	app.Handle("/", middleware.RequireAuth(redirectToLogin)(h.index()))

	mux := http.NewServeMux()
	// Static assets skip the session middleware, so their responses stay
	// cacheable and each asset request doesn't load a session from the store.
	mux.HandleFunc("/static/", h.static)
	mux.Handle("/", middleware.Session(store, sessionOpts)(app))

	return mux
}

// PreloadedState is the state the index template embeds in the page.
type PreloadedState struct {
	CSRFToken string `json:"csrfToken"`

	// Remotes contains those the page registers at startup. It must be
	// non-nil, even when empty, since the page rejects a null list.
	Remotes []Remote `json:"remotes"`
}

// Remote is a reference to a Module Federation remote.
type Remote struct {
	// Name is the module ID prefix.
	Name string `json:"name"`

	// Entry is the manifest URL.
	Entry string `json:"entry"`
}

// renderIndexTemplate writes a 200 HTML response whose body is the index
// template rendered with state. If the template cannot be rendered, a 500 plain
// text response is written instead.
func (h *Handler) renderIndexTemplate(w http.ResponseWriter, r *http.Request, state *PreloadedState) {
	logger := middleware.LoggerFromContext(r.Context())

	var buf bytes.Buffer
	if err := h.indexTemplate.Execute(r.Context(), &buf, state); err != nil {
		logger.Error("failed to execute index template", "err", err)
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	httputil.RenderHTML(w, logger, http.StatusOK, buf.Bytes())
}

var errDevServerResourceNotFound = errors.New("resource not found")

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
