package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

func TestHandler_index(t *testing.T) {
	t.Run("templates the dev server page for a page load in development environment", func(t *testing.T) {
		var devServerPath string
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			devServerPath = r.URL.Path
			w.Header().Set(httputil.HeaderContentType, httputil.MIMETextHTMLCharsetUTF8)
			_, _ = w.Write([]byte(`<html><head><script type="application/json" id="runtime-config">{{.}}</script></head><body><div id="root"></div></body></html>`))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h, err := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
			Remote:       config.RemoteConfig{PostsManifestURL: "https://pg.test/mf-manifest.json"},
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/dashboard?tab=posts", nil)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		want := `<html><head><script type="application/json" id="runtime-config">{"remotes":[{"name":"pg","entry":"https://pg.test/mf-manifest.json"}]}</script></head><body><div id="root"></div></body></html>`
		if got := rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "no-store", rec.Header().Get("Cache-Control"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		// The dev server serves one page for every route, so the shell is
		// always fetched from its root.
		if want, got := "/", devServerPath; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("answers 502 when the dev server page is not a valid template in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(httputil.HeaderContentType, httputil.MIMETextHTMLCharsetUTF8)
			_, _ = w.Write([]byte(`<html>{{</html>`))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h, err := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("proxies everything but a page load in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("proxied:" + r.URL.Path))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h, err := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/mf-manifest.json", nil)
		req.Header.Set("Accept", "*/*")
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "proxied:/mf-manifest.json", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("proxies a form submission in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("io.ReadAll: %v", err)
			}
			_, _ = w.Write([]byte(r.Method + ":" + r.URL.Path + ":" + string(body)))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h, err := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		// A form_post OIDC callback is a browser navigation, so it accepts HTML
		// like a page load does.
		req := httptest.NewRequest(http.MethodPost, "/auth/callback", strings.NewReader("code=abc&state=xyz"))
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "POST:/auth/callback:code=abc&state=xyz", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("answers 502 when the dev server is unreachable in development environment", func(t *testing.T) {
		h, err := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("serves the rendered page for all routes in production environment", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte(`<html><head><script type="application/json" id="runtime-config">{{.}}</script></head><body><div id="root"></div></body></html>`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		h, err := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
			Remote: config.RemoteConfig{
				PostsManifestURL:           "https://pg.test/mf-manifest.json",
				StudentInsightsManifestURL: "https://si.test/mf-manifest.json",
			},
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		want := `<html><head><script type="application/json" id="runtime-config">{"remotes":[{"name":"pg","entry":"https://pg.test/mf-manifest.json"},{"name":"si","entry":"https://si.test/mf-manifest.json"}]}</script></head><body><div id="root"></div></body></html>`
		if got := rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := httputil.MIMETextHTMLCharsetUTF8, rec.Header().Get(httputil.HeaderContentType); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "no-store", rec.Header().Get("Cache-Control"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("embeds an empty array rather than null when no remote is configured", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte(`<script type="application/json" id="runtime-config">{{.}}</script>`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		h, err := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := `<script type="application/json" id="runtime-config">{"remotes":[]}</script>`, rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("answers 500 when the page cannot be rendered in production environment", func(t *testing.T) {
		buildDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(buildDir, "index.html"), []byte(`<html>{{.Missing}}</html>`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		h, err := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("return 404 for an unknown environment", func(t *testing.T) {
		h, err := New(&config.Config{}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h.index(rec, req)

		if want, got := http.StatusNotFound, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})
}

func TestHandler_static(t *testing.T) {
	t.Run("proxy to the dev server in development environment", func(t *testing.T) {
		devServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(httputil.HeaderContentType, httputil.MIMETextHTMLCharsetUTF8)
			_, _ = w.Write([]byte("proxied:" + r.URL.Path))
		}))
		t.Cleanup(devServer.Close)

		devServerURL, err := url.Parse(devServer.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		h, err := New(&config.Config{
			Env:          config.EnvDevelopment,
			DevServerURL: devServerURL,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "proxied:/static/js/index.js", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("serve hashed asset in production environment", func(t *testing.T) {
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

		h, err := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/static/js/index.abc123.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "console.log('Hello world!');", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("return 404 for a directory in production environment", func(t *testing.T) {
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

		h, err := New(&config.Config{
			Env:      config.EnvProduction,
			BuildDir: buildDir,
		}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		tests := []struct {
			name   string
			target string
		}{
			{name: "static root", target: "/static/"},
			{name: "subdirectory", target: "/static/js/"},
			{name: "subdirectory without trailing slash", target: "/static/js"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, tt.target, nil)
				rec := httptest.NewRecorder()

				h.static(rec, req)

				if want, got := http.StatusNotFound, rec.Code; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
			})
		}
	})

	t.Run("return 404 for an unknown environment", func(t *testing.T) {
		h, err := New(&config.Config{}, nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/static/js/missing.js", nil)
		rec := httptest.NewRecorder()

		h.static(rec, req)

		if want, got := http.StatusNotFound, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})
}

func TestNewRuntimeConfig(t *testing.T) {
	t.Run("maps posts to pg and student insights to si in order", func(t *testing.T) {
		got := newRuntimeConfig(config.RemoteConfig{
			PostsManifestURL:           "https://pg.test/mf-manifest.json",
			StudentInsightsManifestURL: "https://si.test/mf-manifest.json",
		})

		want := []runtimeRemote{
			{Name: "pg", Entry: "https://pg.test/mf-manifest.json"},
			{Name: "si", Entry: "https://si.test/mf-manifest.json"},
		}
		if !slices.Equal(want, got.Remotes) {
			t.Errorf("want: %v; got: %v", want, got.Remotes)
		}
	})

	t.Run("skips a remote whose url is empty", func(t *testing.T) {
		got := newRuntimeConfig(config.RemoteConfig{
			StudentInsightsManifestURL: "https://si.test/mf-manifest.json",
		})

		want := []runtimeRemote{{Name: "si", Entry: "https://si.test/mf-manifest.json"}}
		if !slices.Equal(want, got.Remotes) {
			t.Errorf("want: %v; got: %v", want, got.Remotes)
		}
	})

	t.Run("returns an empty slice rather than nil when nothing is configured", func(t *testing.T) {
		got := newRuntimeConfig(config.RemoteConfig{})

		if got.Remotes == nil {
			t.Fatal("want: non-nil; got: nil")
		}
		if want := 0; want != len(got.Remotes) {
			t.Errorf("want: %d; got: %d", want, len(got.Remotes))
		}
	})
}
