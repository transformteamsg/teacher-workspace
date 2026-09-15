package handler

import "testing"

func TestSanitizeReturnTo(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantPath string
		wantOK   bool
	}{
		// Empty / absent
		{name: "returns fallback for empty string", raw: "", wantPath: "/", wantOK: false},

		// Valid same-site paths
		{name: "accepts root path", raw: "/", wantPath: "/", wantOK: true},
		{name: "accepts simple path", raw: "/posts", wantPath: "/posts", wantOK: true},
		{name: "accepts path with query string", raw: "/posts?tab=drafts", wantPath: "/posts?tab=drafts", wantOK: true},
		{name: "accepts path with fragment", raw: "/posts/123#comments", wantPath: "/posts/123#comments", wantOK: true},
		{name: "accepts nested path", raw: "/groups/7/members", wantPath: "/groups/7/members", wantOK: true},
		{name: "accepts path starting with /authentication", raw: "/authentication", wantPath: "/authentication", wantOK: true},
		{name: "accepts path starting with /apikeys", raw: "/apikeys", wantPath: "/apikeys", wantOK: true},

		// Off-site: absolute URLs
		{name: "rejects https absolute URL", raw: "https://evil.example/x", wantPath: "/", wantOK: false},
		{name: "rejects http absolute URL", raw: "http://evil.example", wantPath: "/", wantOK: false},

		// Off-site: protocol-relative
		{name: "rejects protocol-relative URL", raw: "//evil.example/x", wantPath: "/", wantOK: false},

		// Off-site: backslash trick
		{name: "rejects backslash after slash", raw: "/\\evil.example", wantPath: "/", wantOK: false},

		// Off-site: encoded bypasses
		{name: "rejects double-encoded protocol-relative", raw: "%2F%2Fevil.example", wantPath: "/", wantOK: false},
		{name: "rejects lowercase double-encoded", raw: "%2f%2fevil.example", wantPath: "/", wantOK: false},

		// Off-site: no leading slash
		{name: "rejects relative URL without leading slash", raw: "evil.example", wantPath: "/", wantOK: false},

		// Internal route blocking (Q2)
		{name: "rejects /auth/ prefix", raw: "/auth/edupass", wantPath: "/", wantOK: false},
		{name: "rejects /auth/ callback", raw: "/auth/edupass/callback", wantPath: "/", wantOK: false},
		{name: "rejects /Auth/ case bypass", raw: "/Auth/Edupass", wantPath: "/", wantOK: false},
		{name: "rejects /api/ prefix", raw: "/api/posts/hello", wantPath: "/", wantOK: false},
		{name: "rejects /API/ case bypass", raw: "/API/posts", wantPath: "/", wantOK: false},
		{name: "rejects /api/ root", raw: "/api/", wantPath: "/", wantOK: false},

		// Boundary: /auth and /api are blocked by exact match; /authentication and /apikeys are accepted
		{name: "rejects /auth without trailing slash", raw: "/auth", wantPath: "/", wantOK: false},
		{name: "rejects /Auth case bypass without trailing slash", raw: "/Auth", wantPath: "/", wantOK: false},
		{name: "rejects /api without trailing slash", raw: "/api", wantPath: "/", wantOK: false},
		{name: "rejects /API case bypass without trailing slash", raw: "/API", wantPath: "/", wantOK: false},

		// Malformed percent-encoding
		{name: "rejects malformed percent-encoding", raw: "%ZZ", wantPath: "/", wantOK: false},

		// Encoded backslash bypass: %5C decodes to \, caught after QueryUnescape
		{name: "rejects encoded backslash", raw: "/%5Cevil.example", wantPath: "/", wantOK: false},

		// XSS via javascript: scheme -- rejected by no-leading-slash check
		{name: "rejects javascript scheme", raw: "javascript:alert(1)", wantPath: "/", wantOK: false},

		// No leading slash variants
		{name: "rejects query-only string", raw: "?foo=bar", wantPath: "/", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotOK := sanitizeReturnTo(tt.raw)
			if want, got := tt.wantPath, gotPath; want != got {
				t.Errorf("path: want %q; got %q", want, got)
			}
			if want, got := tt.wantOK, gotOK; want != got {
				t.Errorf("ok: want %v; got %v", want, got)
			}
		})
	}
}
