package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
)

func TestHandler_posts(t *testing.T) {
	t.Run("strips the prefix", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("backend:" + r.URL.RequestURI()))
		}))
		t.Cleanup(backend.Close)

		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.posts(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "backend:/hello", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("uses its own backend", func(t *testing.T) {
		posts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("posts:" + r.URL.RequestURI()))
		}))
		t.Cleanup(posts.Close)

		studentInsights := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("student-insights:" + r.URL.RequestURI()))
		}))
		t.Cleanup(studentInsights.Close)

		postsURL, err := url.Parse(posts.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		studentInsightsURL, err := url.Parse(studentInsights.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsURL
		cfg.APIProxy.StudentInsightsBaseURL = studentInsightsURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.posts(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "posts:/hello", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("answers 502 when the backend is unreachable", func(t *testing.T) {
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		backend.Close()

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.posts(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "Bad Gateway\n", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}

func TestHandler_studentInsights(t *testing.T) {
	t.Run("strips the prefix", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("backend:" + r.URL.RequestURI()))
		}))
		t.Cleanup(backend.Close)

		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.StudentInsightsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/student-insights/hello", nil)
		rec := httptest.NewRecorder()

		h.studentInsights(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "backend:/hello", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("uses its own backend", func(t *testing.T) {
		posts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("posts:" + r.URL.RequestURI()))
		}))
		t.Cleanup(posts.Close)

		studentInsights := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("student-insights:" + r.URL.RequestURI()))
		}))
		t.Cleanup(studentInsights.Close)

		postsURL, err := url.Parse(posts.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		studentInsightsURL, err := url.Parse(studentInsights.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = postsURL
		cfg.APIProxy.StudentInsightsBaseURL = studentInsightsURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/student-insights/hello", nil)
		rec := httptest.NewRecorder()

		h.studentInsights(rec, req)

		if want, got := http.StatusOK, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "student-insights:/hello", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("answers 502 when the backend is unreachable", func(t *testing.T) {
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		backend.Close()

		cfg := config.Default()
		cfg.APIProxy.StudentInsightsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/student-insights/hello", nil)
		rec := httptest.NewRecorder()

		h.studentInsights(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := "Bad Gateway\n", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}

func TestHandleProxyError(t *testing.T) {
	t.Run("logs the path the client asked for", func(t *testing.T) {
		var logs bytes.Buffer

		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		backend.Close()

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello", nil)
		rec := httptest.NewRecorder()

		h.posts(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if want, got := `"path":"/api/posts/hello"`, logs.String(); !strings.Contains(got, want) {
			t.Errorf("want logs: containing %q; got: %q", want, got)
		}
	})

	t.Run("keeps the query out of the log", func(t *testing.T) {
		var logs bytes.Buffer

		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		backend.Close()

		cfg := config.Default()
		cfg.APIProxy.PostsBaseURL = backendURL

		h := New(&cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/posts/hello?token=secret", nil)
		rec := httptest.NewRecorder()

		h.posts(rec, req)

		if want, got := http.StatusBadGateway, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if got := logs.String(); strings.Contains(got, "secret") {
			t.Errorf("want logs: without %q; got: %q", "secret", got)
		}
	})
}

func TestHandleProxyNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/unknown-app/hello", nil)
	rec := httptest.NewRecorder()

	handleProxyNotFound(rec, req)

	if want, got := http.StatusNotFound, rec.Code; want != got {
		t.Errorf("want: %d; got: %d", want, got)
	}
	if want, got := "Not Found", rec.Body.String(); want != got {
		t.Errorf("want: %q; got: %q", want, got)
	}
}
