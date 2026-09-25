package htmlutil

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestURLTemplate_Execute(t *testing.T) {
	t.Run("renders the page fetched from the url with the data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>Hello {{.Name}}</html>`))
		}))
		t.Cleanup(server.Close)

		var page bytes.Buffer
		err := NewURLTemplate(server.URL).Execute(t.Context(), &page, struct{ Name string }{Name: "world"})

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "<html>Hello world</html>", page.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("requests the page as html", func(t *testing.T) {
		// The rsbuild development server answers 404 for a request that does
		// not accept HTML.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept"), "text/html") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`<html></html>`))
		}))
		t.Cleanup(server.Close)

		var page bytes.Buffer
		err := NewURLTemplate(server.URL).Execute(t.Context(), &page, nil)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "<html></html>", page.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("returns error when the url is malformed", func(t *testing.T) {
		var page bytes.Buffer
		err := NewURLTemplate("http://%zz").Execute(t.Context(), &page, nil)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "create request"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("returns error when the server is unreachable", func(t *testing.T) {
		var page bytes.Buffer
		err := NewURLTemplate("http://127.0.0.1:1").Execute(t.Context(), &page, nil)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "send request"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("returns error when the server does not answer 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		var page bytes.Buffer
		err := NewURLTemplate(server.URL).Execute(t.Context(), &page, nil)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "server responded 404"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("returns error when the page is not a valid template", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{{Name}}`))
		}))
		t.Cleanup(server.Close)

		var page bytes.Buffer
		err := NewURLTemplate(server.URL).Execute(t.Context(), &page, nil)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "parse template"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("returns error when the context is cancelled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html></html>`))
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		var page bytes.Buffer
		err := NewURLTemplate(server.URL).Execute(ctx, &page, nil)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
	})
}

func TestNewFileTemplate(t *testing.T) {
	t.Run("parses a valid template file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.html")
		if err := os.WriteFile(path, []byte(`<html>{{.Name}}</html>`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		tmpl, err := NewFileTemplate(path)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if tmpl == nil {
			t.Error("want: non-nil; got: nil")
		}
	})

	t.Run("returns error when the file does not exist", func(t *testing.T) {
		_, err := NewFileTemplate(filepath.Join(t.TempDir(), "index.html"))

		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("want err: %v; got: %v", fs.ErrNotExist, err)
		}
	})

	t.Run("returns error when the file is not a valid template", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.html")
		if err := os.WriteFile(path, []byte(`{{Name}}`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		_, err := NewFileTemplate(path)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "parse template"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})
}

func TestFileTemplate_Execute(t *testing.T) {
	t.Run("renders the template with the data", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.html")
		if err := os.WriteFile(path, []byte(`<html>Hello {{.Name}}</html>`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		tmpl, err := NewFileTemplate(path)
		if err != nil {
			t.Fatalf("NewFileTemplate: %v", err)
		}

		var page bytes.Buffer
		err = tmpl.Execute(t.Context(), &page, struct{ Name string }{Name: "world"})

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "<html>Hello world</html>", page.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("returns error when the data does not fit the template", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.html")
		if err := os.WriteFile(path, []byte(`<html>{{.Missing}}</html>`), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		tmpl, err := NewFileTemplate(path)
		if err != nil {
			t.Fatalf("NewFileTemplate: %v", err)
		}

		var page bytes.Buffer
		err = tmpl.Execute(t.Context(), &page, struct{ Name string }{Name: "world"})

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
	})
}
