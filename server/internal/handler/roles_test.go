package handler

import "testing"

func TestEdupassRoleLabel(t *testing.T) {
	t.Run("labels every recognized base role code", func(t *testing.T) {
		codes := []string{
			"ROLE_PRINCIPAL",
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
			"ROLE_ICT_MANAGER",
		}

		for _, code := range codes {
			if !isRecognizedEdupassRole(code) {
				t.Fatalf("want isRecognizedEdupassRole(%q): true; got: false", code)
			}
			if got := edupassRoleLabel(code); got == "" || got == code {
				t.Errorf("want edupassRoleLabel(%q): a label; got: %q", code, got)
			}
		}
	})

	t.Run("returns the code unchanged when it is unrecognized", func(t *testing.T) {
		label := edupassRoleLabel("ROLE_UNKNOWN")

		if want := "ROLE_UNKNOWN"; want != label {
			t.Errorf("want: %q; got: %q", want, label)
		}
	})
}
