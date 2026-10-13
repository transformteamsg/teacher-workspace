package handler

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"strings"
	"testing"
)

// stubTemplate renders a fixed page, for tests that only need to know whether
// a handler rendered the page rather than what it rendered.
type stubTemplate struct{}

func (stubTemplate) Execute(_ context.Context, w io.Writer, _ any) error {
	_, err := io.WriteString(w, "<html>Hello world!</html>")
	return err
}

const (
	preloadedStateOpenTag  = `<script type="application/json" id="preloaded-state">`
	preloadedStateCloseTag = `</script>`
)

// preloadedStateTemplate renders the preloaded state inside a JSON script tag,
// matching where the frontend's page embeds it.
type preloadedStateTemplate struct {
	tmpl *template.Template
}

func newPreloadedStateTemplate() preloadedStateTemplate {
	return preloadedStateTemplate{
		tmpl: template.Must(template.New("preloaded-state").Parse(preloadedStateOpenTag + "{{.}}" + preloadedStateCloseTag)),
	}
}

func (p preloadedStateTemplate) Execute(_ context.Context, w io.Writer, data any) error {
	return p.tmpl.Execute(w, data)
}

// embeddedPreloadedState is the preloaded state as the page embeds it. Remotes
// holds the raw JSON, so a test can tell an empty list from null.
type embeddedPreloadedState struct {
	CSRFToken string          `json:"csrfToken"`
	Remotes   json.RawMessage `json:"remotes"`
}

// decodePreloadedState returns the preloaded state embedded in body, a page
// rendered by [preloadedStateTemplate].
func decodePreloadedState(t *testing.T, body string) embeddedPreloadedState {
	t.Helper()

	payload := strings.TrimSuffix(strings.TrimPrefix(body, preloadedStateOpenTag), preloadedStateCloseTag)

	var state embeddedPreloadedState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	return state
}
