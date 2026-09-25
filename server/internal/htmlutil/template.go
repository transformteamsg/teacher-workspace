// Package htmlutil renders HTML templates, either fetched from a URL on every
// render ([URLTemplate]) or parsed once from a file ([FileTemplate]).
package htmlutil

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

// fetchTimeout bounds a template fetch, so a stalled server fails Execute
// rather than blocking it indefinitely.
const fetchTimeout = 10 * time.Second

// Template is an HTML template that renders with the given data.
type Template interface {
	// Execute renders the template with data into w. ctx bounds any I/O needed
	// to load the template. When Execute returns an error, w may already hold
	// part of the output, so a caller that cannot use partial output should
	// render into a buffer first.
	Execute(ctx context.Context, w io.Writer, data any) error
}

// URLTemplate fetches an HTML template from a URL on every call to Execute, so
// changes to the template take effect on the next call. It is safe for
// concurrent use.
type URLTemplate struct {
	client *http.Client
	url    string
}

// NewURLTemplate returns a [URLTemplate] for the given URL. Nothing is fetched
// until Execute, so an unreachable URL or a malformed template is reported
// there.
func NewURLTemplate(url string) *URLTemplate {
	return &URLTemplate{
		client: &http.Client{Timeout: fetchTimeout},
		url:    url,
	}
}

// Execute fetches the template, parses it, and renders it with the given data.
// Nothing is written to w when the template cannot be fetched or parsed.
func (t *URLTemplate) Execute(ctx context.Context, w io.Writer, data any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	// Development servers such as rsbuild's serve HTML only to a request that
	// accepts it, and Go sends no Accept header of its own.
	req.Header.Set("Accept", httputil.MIMETextHTML)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server responded %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	tmpl, err := template.New(t.url).Parse(string(respBody))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	return tmpl.Execute(w, data)
}

// FileTemplate parses an HTML template once and reuses it for every call to
// Execute. It is safe for concurrent use.
type FileTemplate struct {
	tmpl *template.Template
}

// NewFileTemplate parses the template file at the given path, reporting an
// error when it is missing or malformed.
func NewFileTemplate(path string) (*FileTemplate, error) {
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return &FileTemplate{tmpl: tmpl}, nil
}

// Execute renders the parsed template with the given data.
func (t *FileTemplate) Execute(_ context.Context, w io.Writer, data any) error {
	return t.tmpl.Execute(w, data)
}
