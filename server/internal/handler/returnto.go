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
	// Guards against unbounded session writes.
	if len(raw) > 1024 {
		return "/", false
	}

	// Guards inspect u.Path (decoded); raw is returned as-is to preserve percent-encoding.
	// u.Scheme/u.Host cover absolute URLs. The path prefix check catches bare host names
	// (e.g. evil.example) which have neither scheme nor host.
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return "/", false
	}

	// Browsers treat //host and /\host as off-site redirects.
	if len(u.Path) > 1 && (u.Path[1] == '/' || u.Path[1] == '\\') {
		return "/", false
	}

	// Exact match + slash-prefix: HasPrefix("/auth") alone would also block /authentication and /apikeys.
	lower := strings.ToLower(u.Path)
	if lower == "/auth" || lower == "/api" ||
		strings.HasPrefix(lower, "/auth/") || strings.HasPrefix(lower, "/api/") {
		return "/", false
	}

	return raw, true
}
