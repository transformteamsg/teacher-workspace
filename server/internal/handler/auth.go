package handler

import (
	"net/http"
	"net/url"

	"golang.org/x/oauth2"

	"github.com/String-sg/teacher-workspace/server/internal/edupassrole"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/String-sg/teacher-workspace/server/pkg/random"
)

const (
	sessionKeyOIDCState        = "oidc_state"
	sessionKeyOIDCNonce        = "oidc_nonce"
	sessionKeyOIDCCodeVerifier = "oidc_code_verifier"
	sessionKeyReturnTo         = "return_to"

	loginErrorOAuth2         = "oauth2_failed"
	loginErrorOAuth2Callback = "oauth2_callback_failed"
)

func popSessionString(sess *session.Session, key string) string {
	val, _ := sess.Get(key)
	sess.Delete(key)
	s, _ := val.(string)
	return s
}

func redirectLoginError(w http.ResponseWriter, r *http.Request, errorCode string, returnTo string) {
	q := url.Values{}
	q.Set("error", errorCode)
	if returnTo != "" {
		q.Set("return_to", returnTo)
	}
	http.Redirect(w, r, "/login?"+q.Encode(), http.StatusFound)
}

func (h *Handler) authEdupass(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	var dest string
	if raw := r.URL.Query().Get("return_to"); raw != "" {
		if d, ok := sanitizeReturnTo(raw); ok {
			dest = d
		} else {
			logger.Warn("refused return_to destination", "raw", raw)
		}
	}

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("session not found in context")
		redirectLoginError(w, r, loginErrorOAuth2, dest)
		return
	}

	sess.Set(sessionKeyReturnTo, dest)

	codeVerifier := oauth2.GenerateVerifier()
	nonce := random.Base62(32)
	state := random.Base62(32)

	sess.Set(sessionKeyOIDCState, state)
	sess.Set(sessionKeyOIDCNonce, nonce)
	sess.Set(sessionKeyOIDCCodeVerifier, codeVerifier)

	authURL := h.rp.OAuth2.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(codeVerifier),
		oauth2.SetAuthURLParam("nonce", nonce),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) authEdupassCallback(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("session not found in context")
		redirectLoginError(w, r, loginErrorOAuth2Callback, "")
		return
	}

	storedState := popSessionString(sess, sessionKeyOIDCState)
	storedNonce := popSessionString(sess, sessionKeyOIDCNonce)
	storedVerifier := popSessionString(sess, sessionKeyOIDCCodeVerifier)
	returnTo := popSessionString(sess, sessionKeyReturnTo)

	// fail clears any session the caller already held before this attempt,
	// so a failed re-authentication never leaves an older sign-in's session
	// behind as if it were still current.
	fail := func() {
		sess.Clear()
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		logger.Warn("OIDC provider returned error",
			"error", errParam,
			"error_description", r.URL.Query().Get("error_description"),
		)
		fail()
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		logger.Warn("callback missing state or code")
		fail()
		return
	}

	if storedState == "" {
		logger.Warn("stored state missing from session")
		fail()
		return
	}
	if state != storedState {
		logger.Error("state mismatch")
		fail()
		return
	}

	if storedVerifier == "" {
		logger.Warn("code verifier missing from session")
		fail()
		return
	}

	token, err := h.rp.OAuth2.Exchange(r.Context(), code, oauth2.VerifierOption(storedVerifier))
	if err != nil {
		logger.Error("failed to exchange authorization code", "err", err)
		fail()
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		logger.Error("token response missing id_token")
		fail()
		return
	}

	idToken, err := h.rp.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logger.Error("failed to verify ID token", "err", err)
		fail()
		return
	}

	if storedNonce == "" {
		logger.Warn("stored nonce missing from session")
		fail()
		return
	}
	if idToken.Nonce != storedNonce {
		logger.Error("nonce mismatch")
		fail()
		return
	}

	var claims struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		logger.Error("failed to extract claims", "err", err)
		fail()
		return
	}
	if claims.Email == "" {
		logger.Error("ID token missing email claim")
		fail()
		return
	}

	resolved := edupassrole.Resolve(claims.Groups)
	if len(resolved.Unrecognized) > 0 {
		logger.Warn("discarded unrecognized Edupass role/attribute codes",
			"subject", idToken.Subject,
			"codes", resolved.Unrecognized,
		)
	}
	if len(resolved.Roles) != 1 {
		logger.Warn("staff does not have exactly one recognized base role",
			"subject", idToken.Subject,
			"roles", resolved.Roles,
		)
		fail()
		return
	}

	sess.SetUser(&session.User{
		Email:      claims.Email,
		Role:       resolved.Roles[0],
		Attributes: resolved.Attributes,
	})

	if returnTo == "" {
		returnTo = "/"
	}
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}
