package httputil

// ErrorResponse represents a structured error response.
type ErrorResponse struct {
	Message string `json:"message"`
}

// Error implements [error].
func (e *ErrorResponse) Error() string {
	return e.Message
}
