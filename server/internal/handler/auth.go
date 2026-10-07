package handler

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"

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

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		logger.Warn("OIDC provider returned error",
			"error", errParam,
			"error_description", r.URL.Query().Get("error_description"),
		)
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		logger.Warn("callback missing state or code")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	if storedState == "" {
		logger.Warn("stored state missing from session")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}
	if state != storedState {
		logger.Error("state mismatch")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	if storedVerifier == "" {
		logger.Warn("code verifier missing from session")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	token, err := h.rp.OAuth2.Exchange(r.Context(), code, oauth2.VerifierOption(storedVerifier))
	if err != nil {
		logger.Error("failed to exchange authorization code", "err", err)
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		logger.Error("token response missing id_token")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	idToken, err := h.rp.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logger.Error("failed to verify ID token", "err", err)
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	if storedNonce == "" {
		logger.Warn("stored nonce missing from session")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}
	if idToken.Nonce != storedNonce {
		logger.Error("nonce mismatch")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	var claims struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		logger.Error("failed to extract claims", "err", err)
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}
	if claims.Email == "" {
		logger.Error("ID token missing email claim")
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	resolved := resolveEdupassGroups(claims.Groups)
	if len(resolved.unrecognized) > 0 {
		logger.Warn("discarded unrecognized Edupass role/attribute codes",
			"subject", idToken.Subject,
			"codes", resolved.unrecognized,
		)
	}
	if len(resolved.roles) != 1 {
		logger.Warn("staff does not have exactly one recognized base role",
			"subject", idToken.Subject,
			"roles", resolved.roles,
		)
		redirectLoginError(w, r, loginErrorOAuth2Callback, returnTo)
		return
	}

	sess.SetUser(&session.User{
		Email:      claims.Email,
		Role:       resolved.roles[0],
		Attributes: resolved.attributes,
	})

	if returnTo == "" {
		returnTo = "/"
	}
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

const (
	edupassRoleInfix = "_ROLE_"
	edupassAttrInfix = "_ATTR_"
)

// recognizedEdupassRoles lists every base role code Edupass issues. A staff
// member is meant to hold exactly one for a location, but Edupass enforces
// nothing: resolveEdupassGroups reports every one it sees and leaves the
// "more than one" case to the caller, rather than guessing which was meant.
// Add new codes here as they're recognized.
var recognizedEdupassRoles = map[string]bool{
	"ROLE_PRINCIPAL":                     true,
	"ROLE_VICE_PRINCIPAL":                true,
	"ROLE_VICE_PRINCIPAL_ADMINISTRATION": true,
	"ROLE_ADMIN_MANAGER":                 true,
	"ROLE_ADMIN_SUPPORT":                 true,
	"ROLE_YEAR_HEAD":                     true,
	"ROLE_ASST_YEAR_HEAD":                true,
	"ROLE_HOD":                           true,
	"ROLE_SUBJECT_HEAD":                  true,
	"ROLE_LEVEL_HEAD":                    true,
	"ROLE_SSD":                           true,
	"ROLE_LEAD_TEACHER":                  true,
	"ROLE_SNR_TEACHER":                   true,
	"ROLE_TEACHER":                       true,
	"ROLE_SNR_COUNSELLOR":                true,
	"ROLE_COUNSELLOR":                    true,
	"ROLE_SNR_SEN_OFFICER":               true,
	"ROLE_SEN_OFFICER":                   true,
	"ROLE_SNR_SWO":                       true,
	"ROLE_SWO":                           true,
	"ROLE_AED_TL":                        true,
	"ROLE_ICT_MANAGER":                   true,
}

// recognizedEdupassAttributes lists every attribute code Edupass issues. Add
// new codes here as they're recognized.
var recognizedEdupassAttributes = map[string]bool{
	"ATTR_PG_ADMIN":      true,
	"ATTR_PG_USER":       true,
	"ATTR_CCE":           true,
	"ATTR_DM":            true,
	"ATTR_SDE":           true,
	"ATTR_ECGC":          true,
	"ATTR_WB_SPECIALIST": true,
	"ATTR_WB_TCI":        true,
	"ATTR_SLD":           true,
}

// edupassGroups is the outcome of resolving one staff member's Edupass
// `groups` claim.
type edupassGroups struct {
	// roles contains every recognized base role, stripped of location and
	// environment prefix, in arrival order. More than one entry means
	// Edupass returned conflicting base roles, whether for one location or
	// spread across several.
	roles []string
	// effectiveRole is the sole entry in roles when resolveEdupassGroups
	// found exactly one recognized base role. It's empty when roles holds
	// zero or more than one: neither case has a single role to report.
	effectiveRole string
	// attributes contains every recognized attribute, stripped of location
	// and environment prefix, unranked and in arrival order.
	attributes []string
	// unrecognized contains every raw entry that didn't split into a
	// recognized base role or attribute, verbatim, for logging.
	unrecognized []string
}

// resolveEdupassGroups splits raw (an Edupass `groups` claim) into recognized
// base roles and attributes, stripping the location and environment prefix
// from each.
//
// Edupass mixes two kinds of entries into one array, each prefixed with a
// location code and an environment marker: `<location>_TW_ROLE_<CODE>` for a
// base role and `<location>_TW_ATTR_<CODE>` for an attribute (pre-prod
// Edupass issues `_TWSTG_` instead of `_TW_`). resolveEdupassGroups requires
// `_TW_` or `_TWSTG_` to sit immediately ahead of the `_ROLE_`/`_ATTR_`
// infix, rejecting a lookalike entry meant for a different application; the
// location code ahead of the marker itself is never checked.
//
// An exact duplicate entry is only counted once: Edupass sending the same
// string twice is redundant information, not a second role or attribute. A
// code absent from both reference lists, or not immediately preceded by a
// recognized environment marker, is reported in unrecognized rather than
// blocking resolution: Edupass can add codes between Teacher Workspace
// releases. resolveEdupassGroups doesn't decide whether sign-in proceeds; the
// caller refuses unless roles holds exactly one entry.
func resolveEdupassGroups(raw []string) edupassGroups {
	roles := []string{}
	attributes := []string{}
	unrecognized := []string{}
	seen := map[string]bool{}

	for _, entry := range raw {
		if seen[entry] {
			continue
		}
		seen[entry] = true

		switch {
		case strings.Contains(entry, edupassRoleInfix):
			code := codeAfterInfix(entry, edupassRoleInfix)
			if !fromRecognizedEdupassEnv(entry, edupassRoleInfix) {
				unrecognized = append(unrecognized, entry)
			} else if recognizedEdupassRoles[code] {
				roles = append(roles, code)
			} else {
				unrecognized = append(unrecognized, entry)
			}
		case strings.Contains(entry, edupassAttrInfix):
			code := codeAfterInfix(entry, edupassAttrInfix)
			if !fromRecognizedEdupassEnv(entry, edupassAttrInfix) {
				unrecognized = append(unrecognized, entry)
			} else if recognizedEdupassAttributes[code] {
				attributes = append(attributes, code)
			} else {
				unrecognized = append(unrecognized, entry)
			}
		default:
			unrecognized = append(unrecognized, entry)
		}
	}

	effectiveRole := ""
	if len(roles) == 1 {
		effectiveRole = roles[0]
	}

	return edupassGroups{
		roles:         roles,
		effectiveRole: effectiveRole,
		attributes:    attributes,
		unrecognized:  unrecognized,
	}
}

// codeAfterInfix returns entry with everything ahead of infix removed, the
// infix's own leading underscore dropped, and the rest (its ROLE_/ATTR_
// prefix included) kept as the code.
func codeAfterInfix(entry, infix string) string {
	idx := strings.Index(entry, infix)
	return entry[idx+1:]
}

// fromRecognizedEdupassEnv reports whether entry's prefix, everything ahead
// of infix, ends with a known Edupass environment marker: TW in production,
// TWSTG pre-prod. It rejects a lookalike entry meant for a different
// application that happens to share the ROLE_/ATTR_ infix shape, without
// caring what the location code ahead of the marker itself says.
func fromRecognizedEdupassEnv(entry, infix string) bool {
	prefix := entry[:strings.Index(entry, infix)]
	return strings.HasSuffix(prefix, "_TW") || strings.HasSuffix(prefix, "_TWSTG")
}
