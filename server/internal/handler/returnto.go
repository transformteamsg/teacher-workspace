package handler

import (
	"net/url"
	"strings"
)

// sanitizeReturnTo validates a raw return_to query value and returns (path, ok).
// ok is true when path is safe to redirect to; false when the value was absent, malformed, or refused (path is "/" in all false cases).
func sanitizeReturnTo(raw string) (string, bool) {
	if raw == "" {
		return "/", false
	}

	// Decode before all checks so %2F%2Fevil.com is caught as //evil.com, not passed as-is.
	// Malformed percent-encoding (e.g. %ZZ) cannot be decoded to a known path, so refuse it.
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return "/", false
	}

	// Reject anything that is not a relative path (absolute URLs, schemeless origins, etc.).
	if !strings.HasPrefix(decoded, "/") {
		return "/", false
	}

	// Block protocol-relative URLs: //evil.com and /\evil.com are treated as off-site by browsers.
	if len(decoded) > 1 && (decoded[1] == '/' || decoded[1] == '\\') {
		return "/", false
	}

	// Case-insensitive: /Auth/Edupass and /API/posts must also be refused.
	// Exact match covers the bare /auth and /api paths; slash-prefix covers all sub-paths.
	// HasPrefix("/auth") alone is not used because it would also block /authentication and /apikeys.
	lower := strings.ToLower(decoded)
	if lower == "/auth" || lower == "/api" ||
		strings.HasPrefix(lower, "/auth/") || strings.HasPrefix(lower, "/api/") {
		return "/", false
	}

	// Final guard: if Go's URL parser finds a scheme or host the path encodes one we missed.
	u, err := url.Parse(decoded)
	if err != nil {
		return "/", false
	}
	if u.Scheme != "" || u.Host != "" {
		return "/", false
	}

	return decoded, true
}
