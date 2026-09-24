// Package edupassrole resolves the flat `roles` claim Edupass puts on a
// teacher's ID token into the base roles, effective role, and attributes
// Teacher Workspace stores on the session.
//
// Edupass mixes two kinds of entries into one array, each prefixed with a
// location code and an environment marker: `<location>_TW_ROLE_<CODE>` for a
// base role and `<location>_TW_ATTR_<CODE>` for an attribute (pre-prod
// Edupass issues `_TWSTG_` instead of `_TW_`). Resolve splits on the
// `_ROLE_`/`_ATTR_` infix regardless of what precedes it, so both
// environments' codes resolve identically without extra configuration; token
// verification already keeps the environments apart.
package edupassrole

import "strings"

const (
	roleInfix = "_ROLE_"
	attrInfix = "_ATTR_"
)

// recognizedRoles lists every base role code Edupass issues. A teacher is
// meant to hold exactly one for a location, but Edupass enforces nothing:
// Resolve reports every one it sees and leaves the "more than one" case to
// the caller, rather than guessing which was meant. Add new codes here as
// they're recognized.
var recognizedRoles = map[string]bool{
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

// recognizedAttributes lists every attribute code Edupass issues. Add new
// codes here as they're recognized.
var recognizedAttributes = map[string]bool{
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

// Result is the outcome of resolving one teacher's Edupass `roles` claim.
type Result struct {
	// Roles contains every recognized base role, stripped of location and
	// environment prefix, in arrival order. More than one entry means
	// Edupass returned conflicting base roles, whether for one location or
	// spread across several.
	Roles []string
	// EffectiveRole is the sole entry in Roles when Resolve found exactly
	// one recognized base role. It's empty when Roles holds zero or more
	// than one: neither case has a single role to report.
	EffectiveRole string
	// Attributes contains every recognized attribute, stripped of location
	// and environment prefix, unranked and in arrival order.
	Attributes []string
	// Unrecognized contains every raw entry that didn't split into a
	// recognized base role or attribute, verbatim, for logging.
	Unrecognized []string
}

// Resolve splits raw (an Edupass `roles` claim) into recognized base roles
// and attributes, stripping the location and environment prefix from each.
// An exact duplicate entry is only counted once: Edupass sending the same
// string twice is redundant information, not a second role or attribute. A
// code absent from both reference lists is reported in Unrecognized rather
// than blocking resolution: Edupass can add codes between Teacher Workspace
// releases. Resolve doesn't decide whether sign-in proceeds; the caller
// refuses unless Roles holds exactly one entry.
func Resolve(raw []string) Result {
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
		case strings.Contains(entry, roleInfix):
			code := codeAfterInfix(entry, roleInfix)
			if recognizedRoles[code] {
				roles = append(roles, code)
			} else {
				unrecognized = append(unrecognized, entry)
			}
		case strings.Contains(entry, attrInfix):
			code := codeAfterInfix(entry, attrInfix)
			if recognizedAttributes[code] {
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

	return Result{
		Roles:         roles,
		EffectiveRole: effectiveRole,
		Attributes:    attributes,
		Unrecognized:  unrecognized,
	}
}

// codeAfterInfix returns entry with everything ahead of infix removed, the
// infix's own leading underscore dropped, and the rest (its ROLE_/ATTR_
// prefix included) kept as the code.
func codeAfterInfix(entry, infix string) string {
	idx := strings.Index(entry, infix)
	return entry[idx+1:]
}
