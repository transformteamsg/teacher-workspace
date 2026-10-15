package handler

import "testing"

func TestEdupassRoleLabel(t *testing.T) {
	t.Run("returns the name registered on the onboarding form", func(t *testing.T) {
		tests := []struct {
			name string
			code string
			want string
		}{
			{name: "for ROLE_TEACHER", code: "ROLE_TEACHER", want: "Teacher"},
			{name: "for ROLE_VICE_PRINCIPAL_ADMINISTRATION", code: "ROLE_VICE_PRINCIPAL_ADMINISTRATION", want: "Vice Principal (Administration)"},
			{name: "for ROLE_SNR_SWO", code: "ROLE_SNR_SWO", want: "Senior Student Welfare Officer"},
			{name: "for ROLE_AED_TL", code: "ROLE_AED_TL", want: "Allied Educator (T&L)"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				label := edupassRoleLabel(test.code)

				if test.want != label {
					t.Errorf("want label: %q; got: %q", test.want, label)
				}
			})
		}
	})

	t.Run("labels every recognized base role code", func(t *testing.T) {
		for code := range edupassRoleLabels {
			label := edupassRoleLabel(code)

			if label == "" || label == code {
				t.Errorf("want edupassRoleLabel(%q): a label; got: %q", code, label)
			}
		}
	})

	t.Run("returns the code unchanged when it is unrecognized", func(t *testing.T) {
		label := edupassRoleLabel("ROLE_UNKNOWN")

		if want := "ROLE_UNKNOWN"; want != label {
			t.Errorf("want label: %q; got: %q", want, label)
		}
	})
}
