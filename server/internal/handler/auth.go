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

// logout destroys the session, logging out its user if it has one, and
// redirects to the login page.
//
// It responds with 500 if the request has no session.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	logger := middleware.LoggerFromContext(r.Context())

	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok {
		logger.Error("no session found in context")
		httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		return
	}

	sess.Destroy()

	httputil.Redirect(w, logger, http.StatusSeeOther, "/login")
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

	resolved := resolveEdupassGroups(claims.Groups, h.cfg.Edupass.AppCode)
	if len(resolved.unrecognized) > 0 {
		logger.Warn(
			"discarded unrecognized Edupass role/attribute codes",
			"edupass_id", claims.ID,
			"codes", resolved.unrecognized,
		)
	}
	if len(resolved.roles) != 1 {
		errCode := loginErrNoRole
		switch {
		case len(resolved.roles) > 1:
			errCode = loginErrMultipleRoles
		case resolved.removedMKRole:
			errCode = loginErrUnsupportedSchoolMK
		}

		reason := errCode
		roleCodes := make([]string, 0, len(resolved.roles))
		locations := make([]string, 0, len(resolved.roles))
		for _, role := range resolved.roles {
			roleCodes = append(roleCodes, role.code)
			locations = append(locations, role.location)
			if role.location != resolved.roles[0].location {
				reason = logReasonMultipleLocations
			}
		}

		logger.Warn(
			"staff does not have exactly one recognized base role",
			"reason", reason,
			"edupass_id", claims.ID,
			"roles", roleCodes,
			"locations", locations,
		)
		httputil.Redirect(w, logger, http.StatusFound, loginURL(returnTo, errCode))
		return
	}

	sess.SetUser(session.User{
		ID:         claims.ID,
		Name:       claims.Name,
		Email:      claims.CESEmail,
		Role:       resolved.roles[0].code,
		SchoolCode: resolved.roles[0].location,
		Attributes: resolved.attributes,
	})

	logger.Info("logged in")
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

const (
	edupassRoleInfix = "_ROLE_"
	edupassAttrInfix = "_ATTR_"
)

// edupassRoleLabels maps every Edupass base role code the server recognizes
// to the name shown to users, as Edupass registers it. Recognizing a new code
// means adding it here.
var edupassRoleLabels = map[string]string{
	"ROLE_PRINCIPAL":                     "Principal",
	"ROLE_VICE_PRINCIPAL":                "Vice Principal",
	"ROLE_VICE_PRINCIPAL_ADMINISTRATION": "Vice Principal (Administration)",
	"ROLE_ADMIN_MANAGER":                 "Administration Manager",
	"ROLE_ADMIN_SUPPORT":                 "Administrative Support Staff",
	"ROLE_YEAR_HEAD":                     "Year Head",
	"ROLE_ASST_YEAR_HEAD":                "Assistant Year Head",
	"ROLE_HOD":                           "Head of Department",
	"ROLE_SUBJECT_HEAD":                  "Subject Head",
	"ROLE_LEVEL_HEAD":                    "Level Head",
	"ROLE_SSD":                           "School Staff Developer",
	"ROLE_LEAD_TEACHER":                  "Lead Teacher",
	"ROLE_SNR_TEACHER":                   "Senior Teacher",
	"ROLE_TEACHER":                       "Teacher",
	"ROLE_SNR_COUNSELLOR":                "Senior School Counsellor",
	"ROLE_COUNSELLOR":                    "School Counsellor",
	"ROLE_SNR_SEN_OFFICER":               "Senior SEN Officer",
	"ROLE_SEN_OFFICER":                   "SEN Officer",
	"ROLE_SNR_SWO":                       "Senior Student Welfare Officer",
	"ROLE_SWO":                           "Student Welfare Officer",
	"ROLE_AED_TL":                        "Allied Educator (T&L)",
	"ROLE_ICT_MANAGER":                   "ICT Manager",
}

// edupassRoleLabel returns the name shown to users for an Edupass base role
// code, or "" when the code is unrecognized. authEdupassCallback stores only
// recognized codes, so a stored role always has a label.
func edupassRoleLabel(code string) string {
	return edupassRoleLabels[code]
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

// isEdupassLocationCode reports whether code is a school's location code:
// exactly four ASCII digits.
func isEdupassLocationCode(code string) bool {
	if len(code) != 4 {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

// isMKSchoolCode reports whether code is the location code of a MOE
// Kindergarten (MK) school. The cases are the full list of known MK school
// codes; add new codes here as they're assigned.
func isMKSchoolCode(code string) bool {
	switch code {
	case "6101", "6102", "6103", "6104", "6105", "6106", "6107", "6108", "6109",
		"6110", "6111", "6112", "6113", "6114", "6115", "6116", "6117", "6118", "6119",
		"6120", "6121", "6122", "6123", "6124", "6126", "6127", "6128", "6129",
		"6130", "6131", "6132", "6133", "6134", "6135", "6136", "6137", "6138", "6139",
		"6140", "6141", "6142", "6143", "6144", "6145", "6146", "6147", "6148", "6149",
		"6150", "6151", "6152", "6153", "6154", "6155", "6156", "6157", "6158", "6159":
		return true
	default:
		return false
	}
}

// edupassRole is a base role resolved from one Edupass `groups` entry.
type edupassRole struct {
	// location is the location code the role was granted at.
	location string
	// code is the base role code with the location and app code stripped, such
	// as "ROLE_TEACHER".
	code string
}

// edupassGroups is the outcome of resolving one staff member's Edupass
// `groups` claim.
type edupassGroups struct {
	// roles contains every recognized base role in arrival order. More than
	// one entry means Edupass returned conflicting base roles, whether for
	// one location or spread across several.
	roles []edupassRole
	// attributes contains every recognized attribute, stripped of its
	// location code and app code, unranked and in arrival order.
	attributes []string
	// unrecognized contains every raw entry that didn't split into a
	// recognized base role or attribute, verbatim, for logging. A base role
	// at a MOE Kindergarten (MK) school, or at a location that is not four
	// digits, is reported here too, even though its role code is recognized.
	unrecognized []string
	// removedMKRole reports whether a base role at an MK school was dropped.
	removedMKRole bool
}

// resolveEdupassGroups splits raw (an Edupass `groups` claim) into recognized
// base roles and attributes, stripping the app code from each. A base role
// keeps the location code it was granted at, so a caller can tell roles at one
// location from roles spread across several. An attribute drops its location
// code. Every slice in the result is non-nil, even when empty.
//
// Each entry is an Edupass app role, named
// `<location_code>_<app_code>_<tw_role_code>`. app_code is Teacher Workspace's
// app code for the Edupass environment (TW in production, TWSTG in pre-prod),
// passed in as appCode. Teacher Workspace registers both its base roles and
// its attributes as app roles, so tw_role_code is `<ROLE|ATTR>_<role_code>`:
// `1234_TW_ROLE_TEACHER` is the base role ROLE_TEACHER and `1234_TW_ATTR_CCE`
// the attribute ATTR_CCE, both at location 1234. resolveEdupassGroups requires
// an entry's app_code to be appCode, rejecting a lookalike entry meant for a
// different application or Edupass environment.
//
// A code absent from both reference lists, or in an entry whose app_code is
// not appCode, is reported in unrecognized rather than blocking resolution:
// Edupass can add codes between Teacher Workspace releases. A base role at a
// MOE Kindergarten (MK) school is reported in unrecognized too, since Teacher
// Workspace doesn't support MK staff. The MK check covers base roles only: an
// attribute at an MK school still resolves into attributes, because sign-in is
// granted by a base role alone, so dropping MK base roles is enough to keep
// MK staff out. removedMKRole reports that one was dropped, counting only an
// entry that would otherwise have resolved into roles. A base role whose
// location code is not four digits, such as the global X, is reported in
// unrecognized too, since every Teacher Workspace role is tied to a school.
// resolveEdupassGroups doesn't decide whether sign-in proceeds; the caller
// refuses unless roles holds exactly one entry.
func resolveEdupassGroups(raw []string, appCode config.EdupassAppCode) edupassGroups {
	roles := []edupassRole{}
	attributes := []string{}
	unrecognized := []string{}
	removedMKRole := false

	for _, entry := range raw {
		switch {
		case strings.Contains(entry, edupassRoleInfix):
			code := codeAfterInfix(entry, edupassRoleInfix)
			location, _, _ := strings.Cut(entry, "_")
			_, recognized := edupassRoleLabels[code]
			switch {
			case !hasEdupassAppCode(entry, edupassRoleInfix, appCode) ||
				!recognized ||
				!isEdupassLocationCode(location):
				unrecognized = append(unrecognized, entry)
			case isMKSchoolCode(location):
				unrecognized = append(unrecognized, entry)
				removedMKRole = true
			default:
				roles = append(roles, edupassRole{location: location, code: code})
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
		roles:         roles,
		attributes:    attributes,
		unrecognized:  unrecognized,
		removedMKRole: removedMKRole,
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
