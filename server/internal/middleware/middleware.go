package middleware

import (
	"net/http"
	"slices"
)

type Middleware func(http.Handler) http.Handler

// Chain wraps h in m and returns the composed handler. The first middleware is
// the outermost: it sees the request first and the response last. With no
// middleware, Chain returns h.
func Chain(h http.Handler, m ...Middleware) http.Handler {
	for _, v := range slices.Backward(m) {
		h = v(h)
	}
	return h
}
