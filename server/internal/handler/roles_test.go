package handler

import "testing"

func TestEdupassRoleLabel(t *testing.T) {
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
