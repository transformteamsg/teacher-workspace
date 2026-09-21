package handler

import (
	"net/http"

	"golang.org/x/oauth2"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/String-sg/teacher-workspace/server/pkg/random"
)

const (
	sessionKeyOIDCState        = "oidc_state"
	sessionKeyOIDCNonce        = "oidc_nonce"
	sessionKeyOIDCCodeVerifier = "oidc_code_verifier"
	sessionKeyReturnTo         = "return_to"
)

func popSessionString(sess *session.Session, key string) string {
	val, _ := sess.Get(key)
	sess.Delete(key)
	s, _ := val.(string)
	return s
}

func (h *Handler) authEdupass(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("session not found in context")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	if raw := r.URL.Query().Get("return_to"); raw != "" {
		dest, ok := sanitizeReturnTo(raw)
		if ok {
			sess.Set(sessionKeyReturnTo, dest)
		} else {
			logger.Warn("refused return_to destination", "raw", raw, "resolved", dest)
		}
	}

	codeVerifier := oauth2.GenerateVerifier()
	nonce := random.Base62(32)
	state := random.Base62(32)

	sess.Set(sessionKeyOIDCState, state)
	sess.Set(sessionKeyOIDCNonce, nonce)
	sess.Set(sessionKeyOIDCCodeVerifier, codeVerifier)

	authOpts := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(codeVerifier),
		oauth2.SetAuthURLParam("nonce", nonce),
	}

	//TODO: guard this parameter so it is not available in production
	if account := r.URL.Query().Get("account"); account != "" {
		authOpts = append(authOpts, oauth2.SetAuthURLParam("account", account))
		authOpts = append(authOpts, oauth2.SetAuthURLParam("prompt", "login"))
	}

	authURL := h.rp.OAuth2.AuthCodeURL(state, authOpts...)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) authEdupassCallback(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("session not found in context")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	storedState := popSessionString(sess, sessionKeyOIDCState)
	storedNonce := popSessionString(sess, sessionKeyOIDCNonce)
	storedVerifier := popSessionString(sess, sessionKeyOIDCCodeVerifier)
	returnTo := popSessionString(sess, sessionKeyReturnTo)

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		logger.Warn("OIDC provider returned error",
			"error", errParam,
			"error_description", r.URL.Query().Get("error_description"),
		)
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		logger.Warn("callback missing state or code")
		httputil.RenderPlain(w, logger, http.StatusBadRequest)
		return
	}

	if storedState == "" || state != storedState {
		logger.Warn("state mismatch or missing")
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	if storedVerifier == "" {
		logger.Warn("code verifier missing from session")
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	token, err := h.rp.OAuth2.Exchange(r.Context(), code, oauth2.VerifierOption(storedVerifier))
	if err != nil {
		logger.Error("failed to exchange authorization code", "err", err)
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		logger.Error("token response missing id_token")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	idToken, err := h.rp.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logger.Error("failed to verify ID token", "err", err)
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	if storedNonce == "" || idToken.Nonce != storedNonce {
		logger.Warn("nonce mismatch or missing")
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	var claims struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		logger.Error("failed to extract claims", "err", err)
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}
	if claims.Email == "" {
		logger.Warn("ID token missing email claim")
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	//TODO: remove before PR (debug only)
	logger.Info("user authenticated", "email", claims.Email, "groups", claims.Groups)

	sess.SetUser(&session.User{Email: claims.Email})

	if returnTo == "" {
		returnTo = "/"
	}
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}
