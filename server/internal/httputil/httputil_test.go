package httputil_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/pkg/require"
)

func TestRenderPlain(t *testing.T) {
	t.Run("sets Content-Type header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)
		res := rec.Result()

		require.Equal(t, "text/plain; charset=UTF-8", res.Header.Get("Content-Type"))
	})

	t.Run("sets X-Content-Type-Options header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)
		res := rec.Result()

		require.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
	})

	t.Run("responds with status code from given status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)
		res := rec.Result()

		require.Equal(t, 500, res.StatusCode)
	})

	t.Run("responds with text from given status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)

		require.Equal(t, "Internal Server Error", rec.Body.String())
	})

	t.Run("logs error message", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusNoContent)

		type logLine struct {
			Level    string `json:"level"`
			Renderer string `json:"renderer"`
			Err      string `json:"err"`
		}
		var got logLine
		if err := json.Unmarshal(logs.Bytes(), &got); err != nil {
			t.Fatalf("decoding log output: %v", err)
		}

		require.Equal(t, "ERROR", got.Level)
		require.Equal(t, "plain", got.Renderer)
		require.Equal(t, "http: request method or response status code does not allow body", got.Err)
	})
}

func TestRenderJSON(t *testing.T) {
	t.Run("sets Content-Type header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, nil)
		res := rec.Result()

		require.Equal(t, "application/json; charset=UTF-8", res.Header.Get("Content-Type"))
	})

	t.Run("sets X-Content-Type-Options header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, nil)
		res := rec.Result()

		require.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
	})

	t.Run("responds with status code from given status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, nil)
		res := rec.Result()

		require.Equal(t, 500, res.StatusCode)
	})

	t.Run("responds with JSON from given value", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, httputil.ErrorResponse{Message: "Internal Server Error"})

		require.Equal(t, `{"message":"Internal Server Error"}`+"\n", rec.Body.String())
	})

	t.Run("logs error message", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, func() {})

		type logLine struct {
			Level    string `json:"level"`
			Renderer string `json:"renderer"`
			Err      string `json:"err"`
		}
		var got logLine
		if err := json.Unmarshal(logs.Bytes(), &got); err != nil {
			t.Fatalf("decoding log output: %v", err)
		}

		require.Equal(t, "ERROR", got.Level)
		require.Equal(t, "json", got.Renderer)
		require.Equal(t, "json: unsupported type: func()", got.Err)
	})
}
