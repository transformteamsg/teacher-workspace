package handler

import (
	"reflect"
	"testing"
)

func TestResolveEdupassGroups(t *testing.T) {
	t.Run("resolves a single base role to itself", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "ROLE_TEACHER", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
	})

	t.Run("splits and strips roles and attributes", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_PG_ADMIN", "1234_TW_ATTR_CCE"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"ATTR_PG_ADMIN", "ATTR_CCE"}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves pre-prod codes the same as production codes", func(t *testing.T) {
		prod := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER"})
		stg := resolveEdupassGroups([]string{"1234_TWSTG_ROLE_TEACHER"})

		if want, got := prod.roles, stg.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := prod.effectiveRole, stg.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("leaves effectiveRole empty when no base role is recognized", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ATTR_CCE"})

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{"ATTR_CCE"}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
	})

	t.Run("leaves effectiveRole empty when more than one base role shares a location", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_PRINCIPAL", "1234_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_PRINCIPAL", "ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("leaves effectiveRole empty when base roles span more than one location", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER", "5678_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_TEACHER", "ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("drops and reports an unrecognized code without affecting the rest", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_SUPERINTENDENT"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "ROLE_TEACHER", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{"1234_TW_ROLE_SUPERINTENDENT"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("reports an entry matching neither infix as unrecognized", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_SOMETHING_ELSE"})

		if want, got := []string{"1234_TW_SOMETHING_ELSE"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves an empty claim to an empty result", func(t *testing.T) {
		resolved := resolveEdupassGroups(nil)

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves an empty groups claim the same as a nil one", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{})

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects a role-shaped entry from a different application", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_OTHERAPP_ROLE_TEACHER"})

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"1234_OTHERAPP_ROLE_TEACHER"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects an attribute-shaped entry from a different application", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_OTHERAPP_ATTR_CCE"})

		if want, got := []string{}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{"1234_OTHERAPP_ATTR_CCE"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("rejects a prefix that merely ends in TW without the underscore", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_OTHERTW_ROLE_TEACHER"})

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"1234_OTHERTW_ROLE_TEACHER"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("keeps a base role that isn't at an MK school", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("filters out an MK-anchored role, keeping a valid role elsewhere", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER", "6100_TW_ROLE_HOD"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "ROLE_TEACHER", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{"6100_TW_ROLE_HOD"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("leaves no role when the only one is at an MK school", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"6100_TW_ROLE_TEACHER"})

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := "", resolved.effectiveRole; want != got {
			t.Errorf("effectiveRole: want: %q; got: %q", want, got)
		}
		if want, got := []string{"6100_TW_ROLE_TEACHER"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("keeps an attribute at an MK school, unlike a role there", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"1234_TW_ROLE_TEACHER", "6100_TW_ATTR_CCE"})

		if want, got := []string{"ROLE_TEACHER"}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"ATTR_CCE"}, resolved.attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("attributes: want: %v; got: %v", want, got)
		}
		if want, got := []string{}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})

	t.Run("resolves pre-prod MK codes the same as production codes", func(t *testing.T) {
		resolved := resolveEdupassGroups([]string{"6100_TWSTG_ROLE_TEACHER"})

		if want, got := []string{}, resolved.roles; !reflect.DeepEqual(want, got) {
			t.Errorf("roles: want: %v; got: %v", want, got)
		}
		if want, got := []string{"6100_TWSTG_ROLE_TEACHER"}, resolved.unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("unrecognized: want: %v; got: %v", want, got)
		}
	})
}
