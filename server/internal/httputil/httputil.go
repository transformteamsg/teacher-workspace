// Package httputil provides shared HTTP helpers and constants used across the
// server, such as common header/MIME values and response renderers.
package httputil

import (
	"log/slog"
	"net/http"
)

const (
	HeaderContentType         = "Content-Type"
	HeaderXContentTypeOptions = "X-Content-Type-Options"
)

const (
	charsetUTF8              = "charset=UTF-8"
	MIMETextHTML             = "text/html"
	MIMETextHTMLCharsetUTF8  = MIMETextHTML + "; " + charsetUTF8
	MIMETextPlain            = "text/plain"
	MIMETextPlainCharsetUTF8 = MIMETextPlain + "; " + charsetUTF8
)

// RenderPlain writes a plain text response with the given status code. Write
// failures are logged using the provided logger.
func RenderPlain(w http.ResponseWriter, logger *slog.Logger, status int) {
	w.Header().Set(HeaderContentType, MIMETextPlainCharsetUTF8)
	w.Header().Set(HeaderXContentTypeOptions, "nosniff")

	w.WriteHeader(status)

	if _, err := w.Write([]byte(http.StatusText(status))); err != nil {
		logger.Error("failed to write response body", "renderer", "plain", "err", err)
	}
}
