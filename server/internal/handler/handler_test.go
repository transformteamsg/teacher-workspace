package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io/fs"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/oidc"
)

// newTestClientKey generates an RSA key and a self-signed certificate for it, once per package.
var newTestClientKey = sync.OnceValues(func() (oidc.ClientKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return oidc.ClientKey{}, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "teacher-workspace"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return oidc.ClientKey{}, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return oidc.ClientKey{}, err
	}
	return oidc.ClientKey{Key: key, Cert: cert}, nil
})

// testClientKey returns the key pair test relying parties authenticate with.
func testClientKey(t *testing.T) oidc.ClientKey {
	t.Helper()

	clientKey, err := newTestClientKey()
	if err != nil {
		t.Fatalf("newTestClientKey: %v", err)
	}
	return clientKey
}

func testRP(t *testing.T) *oidc.RelyingParty {
	t.Helper()

	return oidc.New(
		"http://test-issuer",
		"test-client",
		"http://test-issuer/callback",
		"http://test-issuer/authorize",
		"http://test-issuer/token",
		"http://test-issuer/jwks",
		testClientKey(t),
	)
}

func TestNew(t *testing.T) {
	t.Run("fails when index.html is missing in production environment", func(t *testing.T) {
		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = t.TempDir()

		_, err := New(&cfg, testRP(t))

		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("want err: %v; got: %v", fs.ErrNotExist, err)
		}
	})

	t.Run("fails when index.html is not a valid template in production environment", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>{{</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir

		_, err := New(&cfg, testRP(t))

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "parse template"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})
}

func TestHandler_Register(t *testing.T) {
	t.Run("routes to the handler matching the request path", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		postsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("posts:" + r.URL.RequestURI()))
		}))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir
		cfg.APIProxy.PostsBaseURL = postsBackendURL

		h, err := New(&cfg, testRP(t))
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		mux := http.NewServeMux()
		h.Register(mux, func(next http.Handler) http.Handler { return next })

		tests := []struct {
			name     string
			target   string
			wantCode int
			wantBody string
		}{
			{name: "index", target: "/", wantCode: http.StatusOK, wantBody: "<html>Hello world!</html>"},
			{name: "static asset", target: "/static/js/index.abc123.js", wantCode: http.StatusOK, wantBody: "console.log('Hello world!');"},
			{name: "API", target: "/api/posts/hello", wantCode: http.StatusOK, wantBody: "posts:/hello"},
			{name: "API path naming no app", target: "/api/", wantCode: http.StatusNotFound, wantBody: "{\"message\":\"Not Found\"}\n"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				if want, got := tt.wantCode, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := tt.wantBody, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("runs application routes through the session middleware", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		postsBackend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		t.Cleanup(postsBackend.Close)

		postsBackendURL, err := url.Parse(postsBackend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir
		cfg.APIProxy.PostsBaseURL = postsBackendURL

		h, err := New(&cfg, testRP(t))
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		tests := []struct {
			name   string
			target string
		}{
			{name: "index", target: "/"},
			{name: "API", target: "/api/posts/hello"},
			{name: "API path naming no app", target: "/api/"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var calls int

				mux := http.NewServeMux()
				h.Register(mux, func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						next.ServeHTTP(w, r)
					})
				})

				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				// More than one call means the middleware was layered per route
				// rather than once around the sub-mux.
				if want := 1; want != calls {
					t.Errorf("want: %d; got: %d", want, calls)
				}
			})
		}
	})

	t.Run("serves static assets without the session middleware", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(buildDir, "static", "js"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "static", "js", "index.abc123.js"), []byte("console.log('Hello world!');"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte("<html>Hello world!</html>"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		cfg := config.Default()
		cfg.Env = config.EnvProduction
		cfg.BuildDir = buildDir

		h, err := New(&cfg, testRP(t))
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var calls int

		mux := http.NewServeMux()
		h.Register(mux, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				next.ServeHTTP(w, r)
			})
		})

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.abc123.js", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want := 0; want != calls {
			t.Errorf("want: %d; got: %d", want, calls)
		}
	})
}
