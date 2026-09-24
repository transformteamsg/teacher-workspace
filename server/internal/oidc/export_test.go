package oidc

import (
	"testing"
	"time"
)

// SetHTTPTimeout shortens the timeout on calls to the provider until t ends.
func SetHTTPTimeout(t testing.TB, d time.Duration) {
	prev := httpClient.Timeout
	httpClient.Timeout = d
	t.Cleanup(func() { httpClient.Timeout = prev })
}
