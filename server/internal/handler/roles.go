package handler

// edupassRoleLabels maps every Edupass base role code the server recognizes
// to the name shown to users, as the Edupass application onboarding form
// registers it. Recognizing a new code means adding it here.
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
// code. It returns code itself when the code is unrecognized, so a resolved
// role never renders blank.
func edupassRoleLabel(code string) string {
	if label, ok := edupassRoleLabels[code]; ok {
		return label
	}
	return code
}
