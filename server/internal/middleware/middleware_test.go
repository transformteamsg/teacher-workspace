package middleware

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestChain(t *testing.T) {
	t.Run("makes the first middleware the outermost", func(t *testing.T) {
		var events []string
		recordingMiddleware := func(name string) Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					events = append(events, name+" before")
					next.ServeHTTP(w, r)
					events = append(events, name+" after")
				})
			}
		}

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "handler")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		chained := Chain(h, recordingMiddleware("first"), recordingMiddleware("second"), recordingMiddleware("third"))

		chained.ServeHTTP(rec, req)

		if want := []string{
			"first before",
			"second before",
			"third before",
			"handler",
			"third after",
			"second after",
			"first after",
		}; !slices.Equal(want, events) {
			t.Errorf("want events: %q; got: %q", want, events)
		}
	})

	t.Run("returns the handler when no middleware are given", func(t *testing.T) {
		h := http.NewServeMux()

		chained := Chain(h)

		if h != chained {
			t.Errorf("want chained: %p; got: %p", h, chained)
		}
	})
}
