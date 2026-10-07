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
// A code absent from both reference lists, or not immediately preceded by a
// recognized environment marker, is reported in unrecognized rather than
// blocking resolution: Edupass can add codes between Teacher Workspace
// releases. A base role anchored to an MK school's location code is filtered
// the same way; Teacher Workspace doesn't support MK staff, and an attribute
// at an MK school is unaffected. resolveEdupassGroups doesn't decide whether
// sign-in proceeds; the caller refuses unless roles holds exactly one entry.
func resolveEdupassGroups(raw []string) edupassGroups {
	roles := []string{}
	attributes := []string{}
	unrecognized := []string{}

	for _, entry := range raw {
		switch {
		case strings.Contains(entry, edupassRoleInfix):
			code := codeAfterInfix(entry, edupassRoleInfix)
			switch {
			case !fromRecognizedEdupassEnv(entry, edupassRoleInfix):
				unrecognized = append(unrecognized, entry)
			case isMKSchoolCode(locationCode(entry)):
				unrecognized = append(unrecognized, entry)
			case isRecognizedEdupassRole(code):
				roles = append(roles, code)
			default:
				unrecognized = append(unrecognized, entry)
			}
		case strings.Contains(entry, edupassAttrInfix):
			code := codeAfterInfix(entry, edupassAttrInfix)
			if !fromRecognizedEdupassEnv(entry, edupassAttrInfix) {
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

// locationCode returns the digits ahead of the first underscore in entry,
// the location code Edupass prefixes every role and attribute with (e.g.
// "1234" from "1234_TW_ROLE_TEACHER"), regardless of which environment
// marker follows it.
func locationCode(entry string) string {
	before, _, _ := strings.Cut(entry, "_")
	return before
}

// isMKSchoolCode reports whether code is the location code of a MOE
// Kindergarten (MK) school. Teacher Workspace doesn't support MK staff, so
// resolveEdupassGroups filters out a base role entry anchored to one of
// these the same way it filters an unrecognized code. Add new codes here as
// they're recognized.
func isMKSchoolCode(code string) bool {
	switch code {
	case "6100", "6101", "6102", "6103", "6104", "6105", "6106", "6107", "6108", "6109",
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
