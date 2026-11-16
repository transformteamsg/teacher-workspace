// Package edupass interprets values issued by Edupass.
package edupass

import (
	"slices"
	"strings"
)

// AppCode is Teacher Workspace's Edupass app code: the <app_code> in each App
// Role ID Edupass issues it, named `<location_code>_<app_code>_<role_code>`,
// such as "TW" in `1234_TW_ROLE_TEACHER`. Production Edupass issues
// [AppCodeTW], and pre-prod Edupass [AppCodeTWSTG].
type AppCode string

const (
	AppCodeTW    AppCode = "TW"
	AppCodeTWSTG AppCode = "TWSTG"
)

// IsLocationCode reports whether code is a school's location code: exactly
// four ASCII digits.
func IsLocationCode(code string) bool {
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

const (
	roleInfix = "_ROLE_"
	attrInfix = "_ATTR_"
)

// roleLabels maps every Edupass base role code the server recognizes
// to the name shown to users, as Edupass registers it. Recognizing a new code
// means adding it here.
var roleLabels = map[string]string{
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

// RoleLabel returns the name shown to users for an Edupass base role
// code, or "" when the code is unrecognized.
func RoleLabel(code string) string {
	return roleLabels[code]
}

// isRecognizedAttribute reports whether code is an attribute Edupass
// issues. Add new codes here as they're recognized.
func isRecognizedAttribute(code string) bool {
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

// Role is a base role resolved from one Edupass `groups` entry.
type Role struct {
	// Location is the location code the role was granted at.
	Location string
	// Code is the base role code with the location and app code stripped, such
	// as "ROLE_TEACHER".
	Code string
}

// attribute is an attribute resolved from one Edupass `groups` entry,
// before it is checked against the base role's location.
type attribute struct {
	// location is the location code the attribute was granted at.
	location string
	// code is the attribute code with the location and app code stripped, such
	// as "ATTR_CCE".
	code string
	// entry is the raw `groups` entry, kept to report it if it is discarded.
	entry string
}

// Groups is the outcome of resolving one staff member's Edupass
// `groups` claim.
type Groups struct {
	// Roles contains every recognized base role, each once, in arrival order.
	// More than one entry means Edupass returned conflicting base roles, whether
	// for one location or spread across several.
	Roles []Role
	// Attributes contains every recognized attribute granted at the location of
	// the single base role, stripped of its location code and app code, each
	// once, unranked and in arrival order.
	Attributes []string
	// Unrecognized contains, verbatim and for logging, every raw entry that
	// didn't resolve into a recognized base role or attribute. That includes
	// entries whose code is recognized:
	//   - a base role at a MOE Kindergarten (MK) school
	//   - a base role or attribute at a location that is not four digits
	//   - an attribute at a location other than the single base role's
	//   - every attribute when there is not exactly one base role
	Unrecognized []string
	// RemovedMKRole reports whether a base role at an MK school was dropped.
	RemovedMKRole bool
}

// ResolveGroups splits raw (an Edupass `groups` claim) into recognized
// base roles and attributes. Every slice in the result is non-nil.
//
// An entry is `<location_code>_<app_code>_<ROLE|ATTR>_<code>`, such as
// `1234_TW_ROLE_TEACHER` or `1234_TW_ATTR_CCE`. app_code must be appCode (TW in
// production, TWSTG in pre-prod).
//
// A base role keeps its location code, and is kept once however often Edupass
// repeats it. An attribute is kept only when granted at the single base role's
// location, also once however often Edupass repeats it.
//
// These entries are reported in Unrecognized rather than blocking resolution,
// since Edupass can add codes between Teacher Workspace releases:
//   - a code absent from the reference lists
//   - an app_code other than appCode, or extra segments ahead of it, such as
//     1234_5_TW_ROLE_TEACHER
//   - a location that is not four digits, such as the global X
//   - a base role at a MOE Kindergarten (MK) school, which also sets RemovedMKRole
//   - an attribute at a location other than the single base role's
//   - every attribute when there is not exactly one base role
//
// ResolveGroups doesn't decide whether sign-in proceeds: the caller
// refuses unless Roles holds exactly one entry.
func ResolveGroups(raw []string, appCode AppCode) Groups {
	roles := []Role{}
	attributeCandidates := []attribute{}
	unrecognized := []string{}
	removedMKRole := false

	for _, entry := range raw {
		switch {
		case strings.Contains(entry, roleInfix):
			location, app, code := splitEntry(entry, roleInfix)
			_, recognized := roleLabels[code]
			switch {
			case app != string(appCode) ||
				!recognized ||
				!IsLocationCode(location):
				unrecognized = append(unrecognized, entry)
			case isMKSchoolCode(location):
				unrecognized = append(unrecognized, entry)
				removedMKRole = true
			default:
				// A repeat of a role already kept is dropped without being reported.
				role := Role{Location: location, Code: code}
				if !slices.Contains(roles, role) {
					roles = append(roles, role)
				}
			}
		case strings.Contains(entry, attrInfix):
			location, app, code := splitEntry(entry, attrInfix)
			if app != string(appCode) ||
				!isRecognizedAttribute(code) ||
				!IsLocationCode(location) {
				unrecognized = append(unrecognized, entry)
			} else {
				attributeCandidates = append(attributeCandidates, attribute{location: location, code: code, entry: entry})
			}
		default:
			unrecognized = append(unrecognized, entry)
		}
	}

	// An attribute can arrive before its base role, so the location to match is
	// only known once every entry has been seen.
	attributes := []string{}
	for _, candidate := range attributeCandidates {
		if len(roles) != 1 || candidate.location != roles[0].Location {
			unrecognized = append(unrecognized, candidate.entry)
			continue
		}
		// A repeat of an attribute already kept is dropped without being reported.
		if !slices.Contains(attributes, candidate.code) {
			attributes = append(attributes, candidate.code)
		}
	}

	return Groups{
		Roles:         roles,
		Attributes:    attributes,
		Unrecognized:  unrecognized,
		RemovedMKRole: removedMKRole,
	}
}

// splitEntry splits entry, which must contain infix, into the
// `<location>_<app code>_<role code>` parts of an Edupass app role. app is
// everything between the first underscore and the infix, so an extra segment
// ahead of the app code ends up in it. code keeps its ROLE_ or ATTR_ prefix.
func splitEntry(entry, infix string) (location, app, code string) {
	idx := strings.Index(entry, infix)
	location, app, _ = strings.Cut(entry[:idx], "_")
	return location, app, entry[idx+1:]
}
