package handler

// edupassRoleLabel returns the name shown to users for an Edupass base role
// code, as the Edupass application onboarding form registers it. It returns
// code itself when no label is known, so a resolved role never renders blank.
func edupassRoleLabel(code string) string {
	switch code {
	case "ROLE_PRINCIPAL":
		return "Principal"
	case "ROLE_VICE_PRINCIPAL":
		return "Vice Principal"
	case "ROLE_VICE_PRINCIPAL_ADMINISTRATION":
		return "Vice Principal (Administration)"
	case "ROLE_ADMIN_MANAGER":
		return "Administration Manager"
	case "ROLE_ADMIN_SUPPORT":
		return "Administrative Support Staff"
	case "ROLE_YEAR_HEAD":
		return "Year Head"
	case "ROLE_ASST_YEAR_HEAD":
		return "Assistant Year Head"
	case "ROLE_HOD":
		return "Head of Department"
	case "ROLE_SUBJECT_HEAD":
		return "Subject Head"
	case "ROLE_LEVEL_HEAD":
		return "Level Head"
	case "ROLE_SSD":
		return "School Staff Developer"
	case "ROLE_LEAD_TEACHER":
		return "Lead Teacher"
	case "ROLE_SNR_TEACHER":
		return "Senior Teacher"
	case "ROLE_TEACHER":
		return "Teacher"
	case "ROLE_SNR_COUNSELLOR":
		return "Senior School Counsellor"
	case "ROLE_COUNSELLOR":
		return "School Counsellor"
	case "ROLE_SNR_SEN_OFFICER":
		return "Senior SEN Officer"
	case "ROLE_SEN_OFFICER":
		return "SEN Officer"
	case "ROLE_SNR_SWO":
		return "Senior Student Welfare Officer"
	case "ROLE_SWO":
		return "Student Welfare Officer"
	case "ROLE_AED_TL":
		return "Allied Educator (T&L)"
	case "ROLE_ICT_MANAGER":
		return "ICT Manager"
	default:
		return code
	}
}
