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
)

func (h *Handler) authLogin(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("session not found in context")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

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

func (h *Handler) authCallback(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

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

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("session not found in context")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	storedStateAny, _ := sess.Get(sessionKeyOIDCState)
	storedNonceAny, _ := sess.Get(sessionKeyOIDCNonce)
	storedVerifierAny, _ := sess.Get(sessionKeyOIDCCodeVerifier)

	sess.Delete(sessionKeyOIDCState)
	sess.Delete(sessionKeyOIDCNonce)
	sess.Delete(sessionKeyOIDCCodeVerifier)

	storedState, _ := storedStateAny.(string)
	storedNonce, _ := storedNonceAny.(string)
	storedVerifier, _ := storedVerifierAny.(string)

	if storedState == "" || state != storedState {
		logger.Warn("state mismatch or missing")
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

	if idToken.Nonce != storedNonce {
		logger.Warn("nonce mismatch")
		httputil.RenderPlain(w, logger, http.StatusForbidden)
		return
	}

	var claims struct {
		Email string `json:"email"`
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

	sess.SetUser(&session.User{Email: claims.Email})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
