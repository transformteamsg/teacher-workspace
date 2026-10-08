package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/String-sg/teacher-workspace/server/pkg/random"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	sessionKeyEdupassReturnTo     = "edupass_return_to"
	sessionKeyEdupassState        = "edupass_state"
	sessionKeyEdupassNonce        = "edupass_nonce"
	sessionKeyEdupassCodeVerifier = "edupass_code_verifier"
)

// authEdupass starts logging the user in with Edupass. It records a pending
// login in the session for [Handler.authEdupassCallback] to complete, and
// redirects the user to Edupass to log in. Calling it again on the same session
// replaces the pending login.
//
// The return_to query parameter is the path the user lands on after logging in.
// If it is missing or unsafe (see [safeReturnTo]), the user lands on "/".
//
// It responds with 500 if the request has no session.
func (h *Handler) authEdupass(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("no session found in context", "provider", "edupass")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	state := random.Base58(32)
	nonce := random.Base58(32)
	codeVerifier := oauth2.GenerateVerifier()

	sess.Set(sessionKeyEdupassReturnTo, safeReturnTo(r.URL.Query().Get("return_to"), "/"))
	sess.Set(sessionKeyEdupassState, state)
	sess.Set(sessionKeyEdupassNonce, nonce)
	sess.Set(sessionKeyEdupassCodeVerifier, codeVerifier)

	authURL := h.edupassOAuth2Config.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(codeVerifier),
	)

	httputil.Redirect(w, logger, http.StatusFound, authURL)
}

// authEdupassCallback completes the pending login started by
// [Handler.authEdupass]. Edupass redirects the user here after they log in.
//
// On success, it logs the user in to the session with the identity from their
// Edupass ID token, and redirects them to the path they asked to return to.
//
// On failure, it redirects the user to the login page, which reports the
// failure and keeps the path they asked to return to. It fails if:
//   - the session has no pending login
//   - the callback does not belong to the pending login
//   - Edupass reports an error
//   - Edupass does not issue a valid ID token with the claims the session needs
//
// Either way, it clears the pending login from the session, so a repeated
// callback fails because there is no pending login.
//
// It responds with 500 if the request has no session.
func (h *Handler) authEdupassCallback(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("no session found in context", "provider", "edupass")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	returnTo, _ := sess.GetAndDelete[string](sessionKeyEdupassReturnTo)
	if returnTo == "" {
		returnTo = "/"
	}

	state, stateOK := sess.GetAndDelete[string](sessionKeyEdupassState)
	nonce, nonceOK := sess.GetAndDelete[string](sessionKeyEdupassNonce)
	codeVerifier, codeVerifierOK := sess.GetAndDelete[string](sessionKeyEdupassCodeVerifier)

	if !stateOK || !nonceOK || !codeVerifierOK {
		logger.Warn(
			"no pending login in session",
			"provider", "edupass",
			"state", stateOK,
			"nonce", nonceOK,
			"code_verifier", codeVerifierOK,
		)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	query := r.URL.Query()

	if query.Get("state") != state {
		logger.Warn("state mismatch", "provider", "edupass")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	if errCode := query.Get("error"); errCode != "" {
		level := slog.LevelError
		if errCode == "access_denied" {
			// The user declined to log in, or Edupass refused them.
			level = slog.LevelWarn
		}

		attrs := []slog.Attr{
			slog.String("provider", "edupass"),
			slog.String("error_code", errCode),
		}
		if errDescription := query.Get("error_description"); errDescription != "" {
			attrs = append(attrs, slog.String("error_description", errDescription))
		}

		logger.LogAttrs(r.Context(), level, "login failed at provider", attrs...)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	token, err := h.edupassOAuth2Config.Exchange(
		oidc.ClientContext(r.Context(), h.edupassHTTPClient),
		query.Get("code"),
		oauth2.VerifierOption(codeVerifier),
	)
	if err != nil {
		logger.Error("failed to exchange code for token", "provider", "edupass", "err", err)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		logger.Error("no ID token found in token", "provider", "edupass")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	idToken, err := h.edupassIDTokenVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logger.Error("failed to verify ID token", "provider", "edupass", "err", err)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	if idToken.Nonce != nonce {
		logger.Error("nonce mismatch", "provider", "edupass")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		logger.Error("failed to unmarshal claims", "provider", "edupass", "err", err)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}
	if claims.Email == "" {
		logger.Error("no email found in claims", "provider", "edupass")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	sess.SetUser(session.User{Email: claims.Email})

	httputil.Redirect(w, logger, http.StatusFound, returnTo)
}

// safeReturnTo returns candidate if it is safe to redirect to, and fallback
// otherwise. A safe candidate is at most 1024 bytes long, and a redirect to it
// lands on exactly that path on this site.
func safeReturnTo(candidate, fallback string) string {
	if len(candidate) > 1024 {
		return fallback
	}

	if !strings.HasPrefix(candidate, "/") ||
		strings.HasPrefix(candidate, "//") ||
		strings.HasPrefix(candidate, `/\`) {
		return fallback
	}

	u, err := url.Parse(candidate)
	if err != nil {
		return fallback
	}

	if strings.Contains(u.Path, `\`) {
		return fallback
	}

	for segment := range strings.SplitSeq(u.Path, "/") {
		if segment == "." || segment == ".." {
			return fallback
		}
	}

	return candidate
}

// loginFailedURL returns the login page URL for a failed login, carrying
// returnTo.
func loginFailedURL(returnTo string) string {
	q := url.Values{}
	q.Set("error", "oauth2_callback_failed")
	q.Set("return_to", returnTo)
	return "/login?" + q.Encode()
}
