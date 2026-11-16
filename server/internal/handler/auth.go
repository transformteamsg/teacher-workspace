package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/transformteamsg/teacher-workspace/server/internal/config"
	"github.com/transformteamsg/teacher-workspace/server/internal/edupass"
	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
	"github.com/transformteamsg/teacher-workspace/server/internal/middleware"
	"github.com/transformteamsg/teacher-workspace/server/internal/session"
	"github.com/transformteamsg/teacher-workspace/server/pkg/random"
	"golang.org/x/oauth2"
)

// login serves the login page to a user who is not logged in. A user who is
// already logged in is redirected to the return_to query parameter instead,
// or to "/" if it is missing or unsafe (see [safeReturnTo]).
//
// It responds with 500 if the request has no session.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("no session found in context")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	if sess.IsAuthenticated() {
		returnTo := safeReturnTo(r.URL.Query().Get("return_to"), "/")
		httputil.Redirect(w, logger, http.StatusFound, returnTo)
		return
	}

	h.renderIndexTemplate(w, r, &PreloadedState{
		CSRFToken: sess.CSRFToken(),
		Remotes:   []Remote{},
	})
}

// logout returns the handler that destroys the session, logging out its user
// if it has one, and redirects to the Edupass end-session endpoint, which ends
// the Edupass session and returns the user to the login page. It redirects
// whether or not the session has a user, so a user whose Teacher Workspace
// session has already ended still ends their Edupass session.
//
// The handler responds with 500 if the request has no session.
func (h *Handler) logout() http.Handler {
	u := *h.cfg.Edupass.EndSessionURL
	q := u.Query()
	q.Set("post_logout_redirect_uri", h.cfg.Edupass.PostLogoutRedirectURL.String())
	u.RawQuery = q.Encode()
	endSessionURL := u.String()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.LoggerFromContext(r.Context())

		sess, ok := middleware.SessionFromContext(r.Context())
		if !ok {
			logger.Error("no session found in context")
			httputil.RenderPlain(w, logger, http.StatusInternalServerError)
			return
		}

		sess.Destroy()

		httputil.Redirect(w, logger, http.StatusSeeOther, endSessionURL)
	})
}

const (
	sessionKeyEdupassReturnTo     = "edupass_return_to"
	sessionKeyEdupassState        = "edupass_state"
	sessionKeyEdupassNonce        = "edupass_nonce"
	sessionKeyEdupassCodeVerifier = "edupass_code_verifier"
)

const (
	loginErrNoRole               = "no_role"
	loginErrMultipleRoles        = "multiple_roles"
	loginErrUnsupportedSchoolMK  = "unsupported_school_mk"
	loginErrOAuth2CallbackFailed = "oauth2_callback_failed"
)

// logReasonMultipleLocations is the log reason for roles spread across more
// than one location. It has no login error code: staff see the
// [loginErrMultipleRoles] error.
const logReasonMultipleLocations = "multiple_locations"

// authEdupass starts logging the user in with Edupass. It records a pending
// login in the session for [Handler.authEdupassCallback] to complete, and
// redirects the user to Edupass to log in. Calling it again on the same session
// replaces the pending login.
//
// The return_to query parameter is the path the user lands on after logging in.
// If it is missing or unsafe (see [safeReturnTo]), the user lands on "/".
//
// In the development environment, the account query parameter is passed on to
// Edupass with prompt=login, so the mock Edupass signs the user in as that
// account. In production it is ignored.
//
// It responds with 500 if the request has no session.
func (h *Handler) authEdupass(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context()).With("provider", "edupass")

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("no session found in context")
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

	authOpts := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(codeVerifier),
	}

	// account is a mock Edupass parameter. Send it in development only, so a caller
	// can't pass their own parameter through to the real Edupass.
	if h.cfg.Env == config.EnvDevelopment {
		if account := r.URL.Query().Get("account"); account != "" {
			// prompt=login stops the mock Edupass reusing the account from an earlier sign-in.
			authOpts = append(authOpts,
				oauth2.SetAuthURLParam("account", account),
				oauth2.SetAuthURLParam("prompt", "login"),
			)
		}
	}

	authURL := h.edupassOAuth2Config.AuthCodeURL(state, authOpts...)

	httputil.Redirect(w, logger, http.StatusFound, authURL)
}

// authEdupassCallback completes the pending login started by
// [Handler.authEdupass]. Edupass redirects the user here after they log in.
//
// On success, it logs the user in to the session with the identity, base role
// and attributes from their Edupass ID token, and the location code of the
// school the base role was granted at. It then redirects them to the path they
// asked to return to.
//
// On failure, it redirects the user to the login page, which reports the
// failure and keeps the path they asked to return to. The error query
// parameter says why: [loginErrNoRole], [loginErrMultipleRoles] or
// [loginErrUnsupportedSchoolMK] for the role refusals below, and
// [loginErrOAuth2CallbackFailed] for every other failure. It fails if:
//   - the session has no pending login
//   - the callback does not belong to the pending login
//   - Edupass reports an error
//   - Edupass does not issue a valid ID token with the claims the session needs
//   - the ID token's groups claim holds no recognized base role, or more than
//     one, once base roles at MOE Kindergarten (MK) schools are set aside
//
// Either way, it clears the pending login from the session, so a repeated
// callback fails because there is no pending login.
//
// It responds with 500 if the request has no session.
func (h *Handler) authEdupassCallback(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context()).With("provider", "edupass")

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("no session found in context")
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
			"state", stateOK,
			"nonce", nonceOK,
			"code_verifier", codeVerifierOK,
		)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	query := r.URL.Query()

	if query.Get("state") != state {
		logger.Warn("state mismatch")
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
			slog.String("error_code", errCode),
		}
		if errDescription := query.Get("error_description"); errDescription != "" {
			attrs = append(attrs, slog.String("error_description", errDescription))
		}

		logger.LogAttrs(r.Context(), level, "failed to get authorization code", attrs...)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	exchangeOpts := []oauth2.AuthCodeOption{oauth2.VerifierOption(codeVerifier)}
	switch h.cfg.Edupass.ClientAuthMethod {
	case config.OAuth2ClientAuthMethodPrivateKeyJWT:
		now := time.Now()
		clientAssertion := jwt.NewWithClaims(jwt.SigningMethodPS256, jwt.RegisteredClaims{
			Issuer:    h.edupassOAuth2Config.ClientID,
			Subject:   h.edupassOAuth2Config.ClientID,
			Audience:  jwt.ClaimStrings{h.edupassOAuth2Config.Endpoint.TokenURL},
			ID:        random.Base62(32),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		})
		clientAssertion.Header["x5t#S256"] = h.cfg.Edupass.ClientCredentials.CertificateThumbprint

		signedClientAssertion, err := clientAssertion.SignedString(h.cfg.Edupass.ClientCredentials.Key)
		if err != nil {
			logger.Error("failed to sign client assertion", "err", err)
			httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
			return
		}

		exchangeOpts = append(exchangeOpts,
			oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
			oauth2.SetAuthURLParam("client_assertion", signedClientAssertion),
		)
	case config.OAuth2ClientAuthMethodClientSecretPost:
		exchangeOpts = append(exchangeOpts, oauth2.SetAuthURLParam("client_secret", h.cfg.Edupass.ClientCredentials.Secret))
	}

	ctx := oidc.ClientContext(r.Context(), h.edupassHTTPClient)
	token, err := h.edupassOAuth2Config.Exchange(ctx, query.Get("code"), exchangeOpts...)
	if err != nil {
		retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err)
		if !ok {
			logger.Error("failed to exchange code for token", "err", err)
			httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
			return
		}

		var edupassExtensions struct {
			ErrorCodes    []int  `json:"error_codes"`
			Timestamp     string `json:"timestamp"`
			TraceID       string `json:"trace_id"`
			CorrelationID string `json:"correlation_id"`
		}
		_ = json.Unmarshal(retrieveErr.Body, &edupassExtensions)
		logger.Error(
			"failed to exchange code for token",
			"status", retrieveErr.Response.StatusCode,
			"error_code", retrieveErr.ErrorCode,
			"error_description", retrieveErr.ErrorDescription,
			"edupass_error_codes", edupassExtensions.ErrorCodes,
			"edupass_timestamp", edupassExtensions.Timestamp,
			"edupass_trace_id", edupassExtensions.TraceID,
			"edupass_correlation_id", edupassExtensions.CorrelationID,
		)

		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		logger.Error("no ID token found in token")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	idToken, err := h.edupassIDTokenVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logger.Error("failed to verify ID token", "err", err)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	if idToken.Nonce != nonce {
		logger.Error("nonce mismatch")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	// Check iat (issued at) here because the verifier skips it, allowing the same 5 minutes of clock skew it allows for nbf: https://github.com/coreos/go-oidc/blob/v3.21.0/oidc/verify.go#L272-L281
	if idToken.IssuedAt.IsZero() {
		logger.Error("no iat found in ID token")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}
	if idToken.IssuedAt.After(time.Now().Add(5 * time.Minute)) {
		logger.Error("ID token iat is in the future", "iat", idToken.IssuedAt)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	var claims struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		CESEmail string   `json:"cesEmail"`
		Groups   []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		logger.Error("failed to unmarshal claims", "err", err)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}
	if claims.ID == "" {
		logger.Error("no id found in claims")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}
	if claims.Name == "" {
		logger.Error("no name found in claims")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}
	if claims.CESEmail == "" {
		logger.Error("no cesEmail found in claims")
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	logger = logger.With("edupass_id", claims.ID, "email", claims.CESEmail)

	resolved := edupass.ResolveGroups(claims.Groups, h.cfg.Edupass.AppCode)
	if len(resolved.Unrecognized) > 0 {
		logger.Warn(
			"discarded unrecognized Edupass role/attribute codes",
			"codes", resolved.Unrecognized,
		)
	}
	if len(resolved.Roles) != 1 {
		errCode := loginErrNoRole
		switch {
		case len(resolved.Roles) > 1:
			errCode = loginErrMultipleRoles
		case resolved.RemovedMKRole:
			errCode = loginErrUnsupportedSchoolMK
		}

		reason := errCode
		roleCodes := make([]string, 0, len(resolved.Roles))
		locations := make([]string, 0, len(resolved.Roles))
		for _, role := range resolved.Roles {
			roleCodes = append(roleCodes, role.Code)
			locations = append(locations, role.Location)
			if role.Location != resolved.Roles[0].Location {
				reason = logReasonMultipleLocations
			}
		}

		logger.Warn(
			"staff does not have exactly one recognized base role",
			"reason", reason,
			"roles", roleCodes,
			"locations", locations,
			"groups", claims.Groups,
		)
		httputil.Redirect(w, logger, http.StatusFound, loginURL(returnTo, errCode))
		return
	}

	sess.SetUser(session.User{
		ID:         claims.ID,
		Name:       claims.Name,
		Email:      claims.CESEmail,
		Role:       resolved.Roles[0].Code,
		SchoolCode: resolved.Roles[0].Location,
		Attributes: resolved.Attributes,
	})

	logger.Info("user logged in")
	httputil.Redirect(w, logger, http.StatusFound, returnTo)
}

// redirectToLogin redirects the user with 303 to the login page, carrying the
// requested URL to return to.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())
	httputil.Redirect(w, logger, http.StatusSeeOther, loginURL(r.URL.RequestURI(), ""))
}

// renderUnauthorized writes a 401 JSON error response.
func renderUnauthorized(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())
	httputil.RenderJSON(w, logger, http.StatusUnauthorized, &httputil.ErrorResponse{
		Message: http.StatusText(http.StatusUnauthorized),
	})
}

// renderForbidden writes a 403 JSON error response.
func renderForbidden(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())
	httputil.RenderJSON(w, logger, http.StatusForbidden, &httputil.ErrorResponse{
		Message: http.StatusText(http.StatusForbidden),
	})
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

// loginURL returns the login page URL, carrying returnTo and, when errCode is
// not empty, the error code.
func loginURL(returnTo, errCode string) string {
	q := url.Values{}
	q.Set("return_to", returnTo)
	if errCode != "" {
		q.Set("error", errCode)
	}
	return "/login?" + q.Encode()
}

// loginFailedURL returns the login page URL for a failed login, carrying
// returnTo and the generic [loginErrOAuth2CallbackFailed] error code.
func loginFailedURL(returnTo string) string {
	return loginURL(returnTo, loginErrOAuth2CallbackFailed)
}
