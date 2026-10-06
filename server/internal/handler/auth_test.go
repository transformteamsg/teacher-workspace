package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"golang.org/x/oauth2"
)

func TestHandler_authEdupass(t *testing.T) {
	t.Run("responds with 500 when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}
		if got := rec.Header().Get("Location"); got != "" {
			t.Errorf("want Location: empty; got: %q", got)
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := middleware.WithLogger(t.Context(), logger)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "no session found in context", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "edupass", records[0].Provider; want != got {
			t.Errorf("want records[0].Provider: %q; got: %q", want, got)
		}
	})

	t.Run("redirects to the Edupass authorization endpoint", func(t *testing.T) {
		cfg := newEdupassConfig()
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := cfg.Edupass.AuthURL.Scheme, location.Scheme; want != got {
			t.Errorf("want location.Scheme: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.AuthURL.Host, location.Host; want != got {
			t.Errorf("want location.Host: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.AuthURL.Path, location.Path; want != got {
			t.Errorf("want location.Path: %q; got: %q", want, got)
		}
	})

	t.Run("requests an authorization code for the configured client", func(t *testing.T) {
		cfg := newEdupassConfig()
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		query := location.Query()
		if want, got := "code", query.Get("response_type"); want != got {
			t.Errorf("want response_type: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.ClientID, query.Get("client_id"); want != got {
			t.Errorf("want client_id: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.RedirectURL.String(), query.Get("redirect_uri"); want != got {
			t.Errorf("want redirect_uri: %q; got: %q", want, got)
		}
		if want, got := "openid", query.Get("scope"); want != got {
			t.Errorf("want scope: %q; got: %q", want, got)
		}
	})

	t.Run("requests an S256 code challenge", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "S256", location.Query().Get("code_challenge_method"); want != got {
			t.Errorf("want code_challenge_method: %q; got: %q", want, got)
		}
	})

	t.Run("records the state it sends to Edupass in the session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		state, ok := sess.Get[string](sessionKeyEdupassState)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassState) ok: true; got: false")
		}
		if state == "" {
			t.Fatal("want state: non-empty; got: empty")
		}
		if got := location.Query().Get("state"); state != got {
			t.Errorf("want state: %q; got: %q", state, got)
		}
	})

	t.Run("records the nonce it sends to Edupass in the session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		nonce, ok := sess.Get[string](sessionKeyEdupassNonce)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassNonce) ok: true; got: false")
		}
		if nonce == "" {
			t.Fatal("want nonce: non-empty; got: empty")
		}
		if got := location.Query().Get("nonce"); nonce != got {
			t.Errorf("want nonce: %q; got: %q", nonce, got)
		}
	})

	t.Run("records the code verifier for the code challenge it sends to Edupass in the session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		codeVerifier, ok := sess.Get[string](sessionKeyEdupassCodeVerifier)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassCodeVerifier) ok: true; got: false")
		}
		if codeVerifier == "" {
			t.Fatal("want codeVerifier: non-empty; got: empty")
		}
		if want, got := oauth2.S256ChallengeFromVerifier(codeVerifier), location.Query().Get("code_challenge"); want != got {
			t.Errorf("want code_challenge: %q; got: %q", want, got)
		}
	})

	t.Run("records in the session the path the user lands on after logging in", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			target string
			want   string
		}{
			{
				name:   "as the given return_to when it is safe",
				target: "/auth/edupass?return_to=" + url.QueryEscape("/announcements?id=1"),
				want:   "/announcements?id=1",
			},
			{
				name:   `as "/" when return_to is missing`,
				target: "/auth/edupass",
				want:   "/",
			},
			{
				name:   `as "/" when return_to is unsafe`,
				target: "/auth/edupass?return_to=" + url.QueryEscape("//evil.example"),
				want:   "/",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				h, err := New(newEdupassConfig())
				if err != nil {
					t.Fatalf("New: %v", err)
				}

				sess := session.New()
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, sess)

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, test.target, nil)
				rec := httptest.NewRecorder()

				h.authEdupass(rec, req)

				returnTo, ok := sess.Get[string](sessionKeyEdupassReturnTo)
				if !ok {
					t.Fatal("want sess.Get(sessionKeyEdupassReturnTo) ok: true; got: false")
				}
				if want := test.want; want != returnTo {
					t.Errorf("want returnTo: %q; got: %q", want, returnTo)
				}
			})
		}
	})

	t.Run("replaces the pending login when another login starts on the same session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		firstReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass?return_to=%2Ffirst", nil)
		firstRec := httptest.NewRecorder()
		h.authEdupass(firstRec, firstReq)

		secondReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass?return_to=%2Fsecond", nil)
		secondRec := httptest.NewRecorder()
		h.authEdupass(secondRec, secondReq)

		firstLocation, err := firstRec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		secondLocation, err := secondRec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		firstQuery := firstLocation.Query()
		secondQuery := secondLocation.Query()
		for _, param := range []string{"state", "nonce", "code_challenge"} {
			if want, got := firstQuery.Get(param), secondQuery.Get(param); want == got {
				t.Fatalf("want secondQuery.Get(%q): != %q; got: %q", param, want, got)
			}
		}

		state, ok := sess.Get[string](sessionKeyEdupassState)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassState) ok: true; got: false")
		}
		if want := secondQuery.Get("state"); want != state {
			t.Errorf("want state: %q; got: %q", want, state)
		}

		nonce, ok := sess.Get[string](sessionKeyEdupassNonce)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassNonce) ok: true; got: false")
		}
		if want := secondQuery.Get("nonce"); want != nonce {
			t.Errorf("want nonce: %q; got: %q", want, nonce)
		}

		codeVerifier, ok := sess.Get[string](sessionKeyEdupassCodeVerifier)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassCodeVerifier) ok: true; got: false")
		}
		if want, got := secondQuery.Get("code_challenge"), oauth2.S256ChallengeFromVerifier(codeVerifier); want != got {
			t.Errorf("want code challenge of codeVerifier: %q; got: %q", want, got)
		}

		returnTo, ok := sess.Get[string](sessionKeyEdupassReturnTo)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassReturnTo) ok: true; got: false")
		}
		if want := "/second"; want != returnTo {
			t.Errorf("want returnTo: %q; got: %q", want, returnTo)
		}
	})
}

func TestHandler_authEdupassCallback(t *testing.T) {
	t.Run("responds with 500 when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback", nil)
		rec := httptest.NewRecorder()

		h.authEdupassCallback(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}
		if got := rec.Header().Get("Location"); got != "" {
			t.Errorf("want Location: empty; got: %q", got)
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := middleware.WithLogger(t.Context(), logger)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback", nil)
		rec := httptest.NewRecorder()

		h.authEdupassCallback(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "no session found in context", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "edupass", records[0].Provider; want != got {
			t.Errorf("want records[0].Provider: %q; got: %q", want, got)
		}
	})
}

func TestSafeReturnTo(t *testing.T) {
	t.Run("returns the given path", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			candidate string
		}{
			{
				name:      "when it is the root path",
				candidate: "/",
			},
			{
				name:      "when it is a nested path",
				candidate: "/classes/123",
			},
			{
				name:      "when it has a query string",
				candidate: "/classes?tab=students",
			},
			{
				name:      "when it has a fragment",
				candidate: "/classes#roster",
			},
			{
				name:      "when it has a percent-encoded slash in a segment",
				candidate: "/groups/P5%2F3",
			},
			{
				name:      "when its query holds an encoded absolute URL",
				candidate: "/search?q=https%3A%2F%2Fevil.example.com",
			},
			{
				name:      "when its query holds a dot-dot",
				candidate: "/search?q=../classes",
			},
			{
				name:      "when its fragment holds a dot-dot",
				candidate: "/classes#../roster",
			},
			{
				name:      "when its query holds a backslash",
				candidate: `/search?q=a\b`,
			},
			{
				name:      "when it has a trailing slash",
				candidate: "/classes/",
			},
			{
				name:      "when a segment starts with a dot",
				candidate: "/.well-known/security.txt",
			},
			{
				name:      "when a segment starts with a dot-dot",
				candidate: "/files/..draft",
			},
			{
				name:      "when it is exactly 1024 bytes long",
				candidate: "/" + strings.Repeat("a", 1023),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				returnTo := safeReturnTo(test.candidate, "/home")

				if want := test.candidate; want != returnTo {
					t.Errorf("want returnTo: %q; got: %q", want, returnTo)
				}
			})
		}
	})

	t.Run("returns the fallback", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			candidate string
		}{
			{
				name:      "when the given path is empty",
				candidate: "",
			},
			{
				name:      "when the given path is relative",
				candidate: "classes/123",
			},
			{
				name:      "when the given path is a bare host name",
				candidate: "evil.example.com",
			},
			{
				name:      "when the given path is only a query string",
				candidate: "?tab=students",
			},
			{
				name:      "when the given path is an absolute URL",
				candidate: "https://evil.example.com/classes",
			},
			{
				name:      "when the given path has a javascript scheme",
				candidate: "javascript:alert(1)",
			},
			{
				name:      "when the given path is protocol-relative",
				candidate: "//evil.example.com/classes",
			},
			{
				name:      "when the given path is percent-encoded protocol-relative",
				candidate: "%2F%2Fevil.example.com/classes",
			},
			{
				name:      "when the given path starts with a slash and a backslash",
				candidate: `/\evil.example.com/classes`,
			},
			{
				name:      "when the given path has a backslash",
				candidate: `/classes\roster`,
			},
			{
				name:      "when the given path has a percent-encoded backslash",
				candidate: "/%5Cevil.example.com",
			},
			{
				name:      "when the given path is longer than 1024 bytes",
				candidate: "/" + strings.Repeat("a", 1024),
			},
			{
				name:      "when the given path has an invalid percent-encoding",
				candidate: "/classes/%zz",
			},
			{
				name:      "when the given path has a control character",
				candidate: "/\t/evil.example.com/classes",
			},
			{
				name:      "when the given path has a CRLF header injection",
				candidate: "/classes\r\nSet-Cookie: session=evil",
			},
			{
				name:      "when the given path has a dot-dot segment",
				candidate: "/x/../classes",
			},
			{
				name:      "when the given path has consecutive dot-dot segments",
				candidate: "/a/b/../../classes",
			},
			{
				name:      "when the given path has a percent-encoded dot-dot segment",
				candidate: "/x/%2e%2e/classes",
			},
			{
				name:      "when the given path has an uppercase percent-encoded dot-dot segment",
				candidate: "/x/%2E%2E/classes",
			},
			{
				name:      "when the given path has a partly percent-encoded dot-dot segment",
				candidate: "/x/.%2e/classes",
			},
			{
				name:      "when the given path resolves to a protocol-relative path through a dot-dot segment",
				candidate: "/a/..//evil.example.com",
			},
			{
				name:      "when the given path has a dot segment",
				candidate: "/./classes",
			},
			{
				name:      "when the given path resolves to a protocol-relative path through a dot segment",
				candidate: "/.//evil.example.com",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				returnTo := safeReturnTo(test.candidate, "/home")

				if want := "/home"; want != returnTo {
					t.Errorf("want returnTo: %q; got: %q", want, returnTo)
				}
			})
		}
	})
}

// newEdupassConfig returns a development config with every Edupass setting
// filled in, so [New] can build a handler from it.
func newEdupassConfig() *config.Config {
	return &config.Config{
		Env: config.EnvDevelopment,
		DevServerURL: &url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:3001",
		},
		Edupass: config.EdupassConfig{
			IssuerURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
			},
			AuthURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
				Path:   "/oauth2/authorize",
			},
			TokenURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
				Path:   "/oauth2/token",
			},
			JWKSURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
				Path:   "/oauth2/jwks",
			},
			ClientID: "teacher-workspace",
			RedirectURL: &url.URL{
				Scheme: "https",
				Host:   "tw.example.com",
				Path:   "/auth/edupass/callback",
			},
			ClientAuthMethod:  config.EdupassClientAuthMethodClientSecretPost,
			ClientCredentials: config.EdupassClientCredentials{Secret: "teacher-workspace-secret"},
		},
	}
}

// logRecord is one record written by a [slog.JSONHandler].
type logRecord struct {
	Level    string `json:"level"`
	Msg      string `json:"msg"`
	Err      string `json:"err"`
	Provider string `json:"provider"`
}

// decodeLogRecords returns the records in logs, the output of a
// [slog.JSONHandler], in the order they were written.
func decodeLogRecords(t *testing.T, logs *bytes.Buffer) []logRecord {
	t.Helper()

	var records []logRecord
	for line := range bytes.Lines(logs.Bytes()) {
		var record logRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		records = append(records, record)
	}

	return records
}
