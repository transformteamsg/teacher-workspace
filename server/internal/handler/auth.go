package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/httputil"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/String-sg/teacher-workspace/server/pkg/random"
	"github.com/golang-jwt/jwt/v5"
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

type edupassTokenErrorExtensions struct {
	ErrorCodes    []int  `json:"error_codes"`
	Timestamp     string `json:"timestamp"`
	TraceID       string `json:"trace_id"`
	CorrelationID string `json:"correlation_id"`
}

// authEdupassCallback completes the pending login started by
// [Handler.authEdupass]. Edupass redirects the user here after they log in.
//
// On success, it logs the user in to the session with the identity, base role
// and attributes from their Edupass ID token, and redirects them to the path
// they asked to return to.
//
// On failure, it redirects the user to the login page, which reports the
// failure and keeps the path they asked to return to. It fails if:
//   - the session has no pending login
//   - the callback does not belong to the pending login
//   - Edupass reports an error
//   - Edupass does not issue a valid ID token with the claims the session needs
//   - the ID token's groups claim does not hold exactly one recognized base role
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

	exchangeContext := context.WithValue(r.Context(), oauth2.HTTPClient, h.edupassHTTPClient)
	var edupassTokenResponse *oauth2.Token
	var err error
	switch h.cfg.Edupass.ClientAuthMethod {
	case config.OAuth2ClientAuthMethodPrivateKeyJWT:
		issuedAt := time.Now()
		clientAssertion := jwt.NewWithClaims(jwt.SigningMethodPS256, jwt.RegisteredClaims{
			Issuer:    h.edupassOAuth2Config.ClientID,
			Subject:   h.edupassOAuth2Config.ClientID,
			Audience:  jwt.ClaimStrings{h.edupassOAuth2Config.Endpoint.TokenURL},
			ID:        random.Base62(32),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(5 * time.Minute)),
		})
		clientAssertion.Header["x5t#S256"] = h.cfg.Edupass.ClientCredentials.CertificateThumbprint

		signedClientAssertion, signErr := clientAssertion.SignedString(h.cfg.Edupass.ClientCredentials.Key)
		if signErr != nil {
			logger.Error("failed to sign client assertion", "provider", "edupass", "err", signErr)
			httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
			return
		}
		edupassTokenResponse, err = h.edupassOAuth2Config.Exchange(exchangeContext, query.Get("code"),
			oauth2.VerifierOption(codeVerifier),
			oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
			oauth2.SetAuthURLParam("client_assertion", signedClientAssertion),
		)
	case config.OAuth2ClientAuthMethodClientSecretPost:
		edupassTokenResponse, err = h.edupassOAuth2Config.Exchange(exchangeContext, query.Get("code"),
			oauth2.VerifierOption(codeVerifier),
			oauth2.SetAuthURLParam("client_secret", h.cfg.Edupass.ClientCredentials.Secret),
		)
	}
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if !errors.As(err, &retrieveErr) {
			logger.Error("failed to exchange code for token", "provider", "edupass", "err", err)
			httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
			return
		}

		switch retrieveErr.Response.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized:
			var edupassErrorExtensions edupassTokenErrorExtensions
			if err := json.Unmarshal(retrieveErr.Body, &edupassErrorExtensions); err != nil {
				logger.Error("failed to exchange code for token", "provider", "edupass", "status", retrieveErr.Response.StatusCode, "err", err)
			} else {
				logger.Error("failed to exchange code for token",
					"provider", "edupass",
					"status", retrieveErr.Response.StatusCode,
					"error", retrieveErr.ErrorCode,
					"error_description", retrieveErr.ErrorDescription,
					"error_codes", edupassErrorExtensions.ErrorCodes,
					"timestamp", edupassErrorExtensions.Timestamp,
					"trace_id", edupassErrorExtensions.TraceID,
					"correlation_id", edupassErrorExtensions.CorrelationID,
				)
			}
		default:
			logger.Error("failed to exchange code for token", "provider", "edupass", "status", retrieveErr.Response.StatusCode)
		}
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	rawIDToken, ok := edupassTokenResponse.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
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
		ID     string   `json:"id"`
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
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

	resolved := resolveEdupassGroups(claims.Groups, h.cfg.Edupass.AppCode)
	if len(resolved.unrecognized) > 0 {
		logger.Warn(
			"discarded unrecognized Edupass role/attribute codes",
			"edupass_id", claims.ID,
			"codes", resolved.unrecognized,
		)
	}
	if len(resolved.roles) != 1 {
		logger.Warn(
			"staff does not have exactly one recognized base role",
			"edupass_id", claims.ID,
			"roles", resolved.roles,
		)
		httputil.Redirect(w, logger, http.StatusFound, loginFailedURL(returnTo))
		return
	}

	sess.SetUser(session.User{
		Email:      claims.Email,
		Role:       resolved.roles[0],
		Attributes: resolved.attributes,
	})

	logger.Info("logged in")
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

const (
	edupassRoleInfix = "_ROLE_"
	edupassAttrInfix = "_ATTR_"
)

// isRecognizedEdupassRole reports whether code is a base role Edupass
// issues. A staff member is meant to hold exactly one for a location, but
// Edupass enforces nothing: resolveEdupassGroups reports every one it sees
// and leaves the "more than one" case to the caller, rather than guessing
// which was meant. Add new codes here as they're recognized.
func isRecognizedEdupassRole(code string) bool {
	switch code {
	case "ROLE_PRINCIPAL",
		"ROLE_VICE_PRINCIPAL",
		"ROLE_VICE_PRINCIPAL_ADMINISTRATION",
		"ROLE_ADMIN_MANAGER",
		"ROLE_ADMIN_SUPPORT",
		"ROLE_YEAR_HEAD",
		"ROLE_ASST_YEAR_HEAD",
		"ROLE_HOD",
		"ROLE_SUBJECT_HEAD",
		"ROLE_LEVEL_HEAD",
		"ROLE_SSD",
		"ROLE_LEAD_TEACHER",
		"ROLE_SNR_TEACHER",
		"ROLE_TEACHER",
		"ROLE_SNR_COUNSELLOR",
		"ROLE_COUNSELLOR",
		"ROLE_SNR_SEN_OFFICER",
		"ROLE_SEN_OFFICER",
		"ROLE_SNR_SWO",
		"ROLE_SWO",
		"ROLE_AED_TL",
		"ROLE_ICT_MANAGER":
		return true
	default:
		return false
	}
}

// isRecognizedEdupassAttribute reports whether code is an attribute Edupass
// issues. Add new codes here as they're recognized.
func isRecognizedEdupassAttribute(code string) bool {
	switch code {
	case "ATTR_PG_ADMIN",
		"ATTR_PG_USER",
		"ATTR_CCE",
		"ATTR_DM",
		"ATTR_SDE",
		"ATTR_ECGC",
		"ATTR_WB_SPECIALIST",
		"ATTR_WB_TCI",
		"ATTR_SLD":
		return true
	default:
		return false
	}
}

// edupassGroups is the outcome of resolving one staff member's Edupass
// `groups` claim.
type edupassGroups struct {
	// roles contains every recognized base role, stripped of its location
	// code and app code, in arrival order. More than one entry means
	// Edupass returned conflicting base roles, whether for one location or
	// spread across several.
	roles []string
	// attributes contains every recognized attribute, stripped of its
	// location code and app code, unranked and in arrival order.
	attributes []string
	// unrecognized contains every raw entry that didn't split into a
	// recognized base role or attribute, verbatim, for logging.
	unrecognized []string
}

// resolveEdupassGroups splits raw (an Edupass `groups` claim) into recognized
// base roles and attributes, stripping the location code and app code from
// each. Every slice in the result is non-nil, even when empty.
//
// Each entry is an Edupass app role, named
// `<location_code>_<app_code>_<tw_role_code>`. app_code is Teacher Workspace's
// app code for the Edupass environment (TW in production, TWSTG in pre-prod),
// passed in as appCode. Teacher Workspace registers both its base roles and
// its attributes as app roles, so tw_role_code is `<ROLE|ATTR>_<role_code>`:
// `1234_TW_ROLE_TEACHER` is the base role ROLE_TEACHER and `1234_TW_ATTR_CCE`
// the attribute ATTR_CCE, both at location 1234. resolveEdupassGroups requires
// an entry's app_code to be appCode, rejecting a lookalike entry meant for a
// different application or Edupass environment; the location code is never
// checked.
//
// A code absent from both reference lists, or in an entry whose app_code is
// not appCode, is reported in unrecognized rather than blocking resolution:
// Edupass can add codes between Teacher Workspace releases.
// resolveEdupassGroups doesn't decide whether sign-in proceeds; the caller
// refuses unless roles holds exactly one entry.
func resolveEdupassGroups(raw []string, appCode config.EdupassAppCode) edupassGroups {
	roles := []string{}
	attributes := []string{}
	unrecognized := []string{}

	for _, entry := range raw {
		switch {
		case strings.Contains(entry, edupassRoleInfix):
			code := codeAfterInfix(entry, edupassRoleInfix)
			if !hasEdupassAppCode(entry, edupassRoleInfix, appCode) {
				unrecognized = append(unrecognized, entry)
			} else if isRecognizedEdupassRole(code) {
				roles = append(roles, code)
			} else {
				unrecognized = append(unrecognized, entry)
			}
		case strings.Contains(entry, edupassAttrInfix):
			code := codeAfterInfix(entry, edupassAttrInfix)
			if !hasEdupassAppCode(entry, edupassAttrInfix, appCode) {
				unrecognized = append(unrecognized, entry)
			} else if isRecognizedEdupassAttribute(code) {
				attributes = append(attributes, code)
			} else {
				unrecognized = append(unrecognized, entry)
			}
		default:
			unrecognized = append(unrecognized, entry)
		}
	}

	return edupassGroups{
		roles:        roles,
		attributes:   attributes,
		unrecognized: unrecognized,
	}
}

// codeAfterInfix returns entry with everything ahead of infix removed, the
// infix's own leading underscore dropped, and the rest (its ROLE_/ATTR_
// prefix included) kept as the code.
func codeAfterInfix(entry, infix string) string {
	idx := strings.Index(entry, infix)
	return entry[idx+1:]
}

// hasEdupassAppCode reports whether entry's app code, the underscore-separated
// part just ahead of infix, is appCode. It rejects a lookalike entry meant for
// a different application or Edupass environment that happens to share the
// ROLE_/ATTR_ infix shape, without caring what the location code ahead of the
// app code says.
func hasEdupassAppCode(entry, infix string, appCode config.EdupassAppCode) bool {
	prefix := entry[:strings.Index(entry, infix)]
	return strings.HasSuffix(prefix, "_"+string(appCode))
}
