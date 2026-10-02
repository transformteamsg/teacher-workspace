package edupassrole

import (
	"reflect"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Run("resolves a single base role to itself", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "ROLE_TEACHER", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("Attributes: want: %v; got: %v", want, got)
		}
	})

	t.Run("splits and strips roles and attributes", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_PG_ADMIN", "1234_TW_ATTR_CCE"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"ATTR_PG_ADMIN", "ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("Attributes: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves pre-prod codes the same as production codes", func(t *testing.T) {
		prod := Resolve([]string{"1234_TW_ROLE_TEACHER"})
		stg := Resolve([]string{"1234_TWSTG_ROLE_TEACHER"})

		if want, got := prod.Roles, stg.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := prod.EffectiveRole, stg.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("leaves EffectiveRole empty when no base role is recognized", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ATTR_CCE"})

		if want, got := []string{}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{"ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("Attributes: want: %v; got: %v", want, got)
		}
	})

	t.Run("leaves EffectiveRole empty when more than one base role shares a location", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ROLE_PRINCIPAL", "1234_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_PRINCIPAL", "ROLE_TEACHER"}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("counts an exact duplicate entry only once", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "ROLE_TEACHER", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("leaves EffectiveRole empty when base roles span more than one location", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ROLE_TEACHER", "5678_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_TEACHER", "ROLE_TEACHER"}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("drops and reports an unrecognized code without affecting the rest", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_SUPERINTENDENT"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "ROLE_TEACHER", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{"1234_TW_ROLE_SUPERINTENDENT"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("reports an entry matching neither infix as unrecognized", func(t *testing.T) {
		resolved := Resolve([]string{"1234_TW_SOMETHING_ELSE"})

		if want, got := []string{"1234_TW_SOMETHING_ELSE"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves an empty claim to an empty result", func(t *testing.T) {
		resolved := Resolve(nil)

		if want, got := []string{}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("Attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves an empty groups claim the same as a nil one", func(t *testing.T) {
		resolved := Resolve([]string{})

		if want, got := []string{}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("Attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects a role-shaped entry from a different application", func(t *testing.T) {
		resolved := Resolve([]string{"1234_OTHERAPP_ROLE_TEACHER"})

		if want, got := []string{}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"1234_OTHERAPP_ROLE_TEACHER"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects an attribute-shaped entry from a different application", func(t *testing.T) {
		resolved := Resolve([]string{"1234_OTHERAPP_ATTR_CCE"})

		if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("Attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{"1234_OTHERAPP_ATTR_CCE"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects a prefix that merely ends in TW without the underscore", func(t *testing.T) {
		resolved := Resolve([]string{"1234_OTHERTW_ROLE_TEACHER"})

		if want, got := []string{}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("Roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"1234_OTHERTW_ROLE_TEACHER"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got)
		}
	})
}
