package edupassrole

import (
	"reflect"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Run("resolves a single base role to itself", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ROLE_TEACHER"})

		if want := []string{"ROLE_TEACHER"}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "ROLE_TEACHER", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want := []string{}; !reflect.DeepEqual(want, got.Attributes) {
			t.Errorf("Attributes: want: %v; got: %v", want, got.Attributes)
		}
	})

	t.Run("splits and strips roles and attributes", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_PG_ADMIN", "1234_TW_ATTR_CCE"})

		if want := []string{"ROLE_TEACHER"}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want := []string{"ATTR_PG_ADMIN", "ATTR_CCE"}; !reflect.DeepEqual(want, got.Attributes) {
			t.Errorf("Attributes: want: %v; got: %v", want, got.Attributes)
		}
	})

	t.Run("resolves pre-prod codes the same as production codes", func(t *testing.T) {
		prod := Resolve([]string{"1234_TW_ROLE_TEACHER"})
		stg := Resolve([]string{"1234_TWSTG_ROLE_TEACHER"})

		if !reflect.DeepEqual(prod.Roles, stg.Roles) {
			t.Errorf("Roles: want: %v; got: %v", prod.Roles, stg.Roles)
		}
		if prod.EffectiveRole != stg.EffectiveRole {
			t.Errorf("EffectiveRole: want: %q; got: %q", prod.EffectiveRole, stg.EffectiveRole)
		}
	})

	t.Run("leaves EffectiveRole empty when no base role is recognized", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ATTR_CCE"})

		if want := []string{}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want := []string{"ATTR_CCE"}; !reflect.DeepEqual(want, got.Attributes) {
			t.Errorf("Attributes: want: %v; got: %v", want, got.Attributes)
		}
	})

	t.Run("leaves EffectiveRole empty when more than one base role shares a location", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ROLE_PRINCIPAL", "1234_TW_ROLE_TEACHER"})

		if want := []string{"ROLE_PRINCIPAL", "ROLE_TEACHER"}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("counts an exact duplicate entry only once", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_TEACHER"})

		if want := []string{"ROLE_TEACHER"}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "ROLE_TEACHER", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("leaves EffectiveRole empty when base roles span more than one location", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ROLE_TEACHER", "5678_TW_ROLE_TEACHER"})

		if want := []string{"ROLE_TEACHER", "ROLE_TEACHER"}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
	})

	t.Run("drops and reports an unrecognized code without affecting the rest", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_SUPERINTENDENT"})

		if want := []string{"ROLE_TEACHER"}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "ROLE_TEACHER", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want := []string{"1234_TW_ROLE_SUPERINTENDENT"}; !reflect.DeepEqual(want, got.Unrecognized) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got.Unrecognized)
		}
	})

	t.Run("reports an entry matching neither infix as unrecognized", func(t *testing.T) {
		got := Resolve([]string{"1234_TW_SOMETHING_ELSE"})

		if want := []string{"1234_TW_SOMETHING_ELSE"}; !reflect.DeepEqual(want, got.Unrecognized) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got.Unrecognized)
		}
	})

	t.Run("resolves an empty claim to an empty result", func(t *testing.T) {
		got := Resolve(nil)

		if want := []string{}; !reflect.DeepEqual(want, got.Roles) {
			t.Errorf("Roles: want: %v; got: %v", want, got.Roles)
		}
		if want, got := "", got.EffectiveRole; want != got {
			t.Errorf("EffectiveRole: want: %q; got: %q", want, got)
		}
		if want := []string{}; !reflect.DeepEqual(want, got.Attributes) {
			t.Errorf("Attributes: want: %v; got: %v", want, got.Attributes)
		}
		if want := []string{}; !reflect.DeepEqual(want, got.Unrecognized) {
			t.Errorf("Unrecognized: want: %v; got: %v", want, got.Unrecognized)
		}
	})
}
