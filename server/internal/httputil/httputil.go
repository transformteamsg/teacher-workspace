// Package httputil provides shared HTTP helpers and constants used across the
// server, such as common header/MIME values and response renderers.
package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

const (
	HeaderContentType         = "Content-Type"
	HeaderXContentTypeOptions = "X-Content-Type-Options"
	HeaderLocation            = "Location"
)

const (
	charsetUTF8                    = "charset=UTF-8"
	MIMETextHTML                   = "text/html"
	MIMETextHTMLCharsetUTF8        = MIMETextHTML + "; " + charsetUTF8
	MIMETextPlain                  = "text/plain"
	MIMETextPlainCharsetUTF8       = MIMETextPlain + "; " + charsetUTF8
	MIMEApplicationJSON            = "application/json"
	MIMEApplicationJSONCharsetUTF8 = MIMEApplicationJSON + "; " + charsetUTF8
)

// RenderPlain writes a plain text response with the given status code, whose
// body is the status text ([http.StatusText]). The response sets
// X-Content-Type-Options: nosniff, so browsers treat the body only as plain
// text. If the body cannot be written, the error is logged to logger.
func RenderPlain(w http.ResponseWriter, logger *slog.Logger, status int) {
	w.Header().Set(HeaderContentType, MIMETextPlainCharsetUTF8)
	w.Header().Set(HeaderXContentTypeOptions, "nosniff")

	w.WriteHeader(status)

	if _, err := w.Write([]byte(http.StatusText(status))); err != nil {
		logger.Error("failed to write response body", "renderer", "plain", "err", err)
	}
}

// RenderHTML writes an HTML response with the given status code and body. The
// response sets X-Content-Type-Options: nosniff, so browsers treat the body only
// as HTML. If the body cannot be written, the error is logged to logger.
func RenderHTML(w http.ResponseWriter, logger *slog.Logger, status int, body []byte) {
	w.Header().Set(HeaderContentType, MIMETextHTMLCharsetUTF8)
	w.Header().Set(HeaderXContentTypeOptions, "nosniff")

	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		logger.Error("failed to write response body", "renderer", "html", "err", err)
	}
}

// RenderJSON writes a JSON response with the given status code, whose body is v
// encoded as JSON. The response sets X-Content-Type-Options: nosniff, so
// browsers treat the body only as JSON. If v cannot be encoded, a 500 plain text
// response is written instead. If the body cannot be encoded or written, the
// error is logged to logger.
func RenderJSON(w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		logger.Error("failed to encode response body", "renderer", "json", "err", err)
		RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	w.Header().Set(HeaderContentType, MIMEApplicationJSONCharsetUTF8)
	w.Header().Set(HeaderXContentTypeOptions, "nosniff")

	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		logger.Error("failed to write response body", "renderer", "json", "err", err)
	}
}

// Redirect redirects the client to url with the given status code. url is sent
// in the Location header as given, not resolved against the request URL. status
// must be 301, 302, 303, 307, or 308: any other status is logged to logger as
// an error, and a 500 plain text response is written instead.
func Redirect(w http.ResponseWriter, logger *slog.Logger, status int, url string) {
	switch status {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
	default:
		logger.Error("invalid status code for redirect", "status", status)
		RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	w.Header().Set(HeaderLocation, url)
	w.WriteHeader(status)
}
