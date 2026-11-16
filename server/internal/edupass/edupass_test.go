package edupass

import (
	"reflect"
	"testing"
)

func TestResolveGroups(t *testing.T) {
	t.Run("resolves a single base role to its code and location", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER"}, AppCodeTW)

		if want, got := []Role{{Location: "1234", Code: "ROLE_TEACHER"}}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
		if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("splits roles and attributes and strips the app code", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_PG_ADMIN", "1234_TW_ATTR_CCE"}, AppCodeTW)

		if want, got := []Role{{Location: "1234", Code: "ROLE_TEACHER"}}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
		if want, got := []string{"ATTR_PG_ADMIN", "ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("resolves pre-prod codes with the pre-prod app code the same as production codes with the production app code", func(t *testing.T) {
		prod := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_CCE"}, AppCodeTW)
		stg := ResolveGroups([]string{"1234_TWSTG_ROLE_TEACHER", "1234_TWSTG_ATTR_CCE"}, AppCodeTWSTG)

		if !reflect.DeepEqual(prod, stg) {
			t.Errorf("want stg: %+v; got: %+v", prod, stg)
		}
	})

	t.Run("resolves no base role when none is recognized", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ATTR_CCE"}, AppCodeTW)

		if want, got := []Role{}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
	})

	t.Run("resolves every base role", func(t *testing.T) {
		for _, test := range []struct {
			name string
			raw  []string
			want []Role
		}{
			{
				name: "when more than one shares a location",
				raw:  []string{"1234_TW_ROLE_PRINCIPAL", "1234_TW_ROLE_TEACHER"},
				want: []Role{
					{Location: "1234", Code: "ROLE_PRINCIPAL"},
					{Location: "1234", Code: "ROLE_TEACHER"},
				},
			},
			{
				name: "when they span more than one location",
				raw:  []string{"1234_TW_ROLE_TEACHER", "5678_TW_ROLE_TEACHER"},
				want: []Role{
					{Location: "1234", Code: "ROLE_TEACHER"},
					{Location: "5678", Code: "ROLE_TEACHER"},
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups(test.raw, AppCodeTW)

				if want, got := test.want, resolved.Roles; !reflect.DeepEqual(want, got) {
					t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
				}
			})
		}
	})

	t.Run("drops and reports an unrecognized code without affecting the rest", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_SUPERINTENDENT"}, AppCodeTW)

		if want, got := []Role{{Location: "1234", Code: "ROLE_TEACHER"}}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
		if want, got := []string{"1234_TW_ROLE_SUPERINTENDENT"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("treats a base role at an MK school as unrecognized", func(t *testing.T) {
		resolved := ResolveGroups([]string{"6101_TW_ROLE_HOD", "1234_TW_ROLE_TEACHER"}, AppCodeTW)

		if want, got := []Role{{Location: "1234", Code: "ROLE_TEACHER"}}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
		if want, got := []string{"6101_TW_ROLE_HOD"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("reports a base role whose location is not four digits as unrecognized", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			entry string
		}{
			{
				name:  "when the location is the global X",
				entry: "X_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is three digits",
				entry: "123_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is five digits",
				entry: "12345_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is letters",
				entry: "ABCD_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is four digits mixed with a letter",
				entry: "123A_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is full-width digits",
				entry: "\uFF11\uFF12\uFF13\uFF14_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is four bytes long with a non-ASCII digit",
				entry: "12\u0663_TW_ROLE_TEACHER",
			},
			{
				name:  "when the location is missing",
				entry: "_TW_ROLE_TEACHER",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups([]string{test.entry}, AppCodeTW)

				want := Groups{
					Roles:        []Role{},
					Attributes:   []string{},
					Unrecognized: []string{test.entry},
				}
				if !reflect.DeepEqual(want, resolved) {
					t.Errorf("want resolved: %+v; got: %+v", want, resolved)
				}
			})
		}
	})

	t.Run("reports a base role at a global location without affecting a valid one", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", "X_TW_ROLE_PRINCIPAL"}, AppCodeTW)

		if want, got := []Role{{Location: "1234", Code: "ROLE_TEACHER"}}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
		if want, got := []string{"X_TW_ROLE_PRINCIPAL"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("reports a base role with an extra segment ahead of the app code as unrecognized", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			entry string
		}{
			{
				name:  "at a school",
				entry: "1234_5678_TW_ROLE_TEACHER",
			},
			{
				name:  "at an MK school",
				entry: "6101_5678_TW_ROLE_TEACHER",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups([]string{test.entry}, AppCodeTW)

				want := Groups{Roles: []Role{}, Attributes: []string{}, Unrecognized: []string{test.entry}}
				if !reflect.DeepEqual(want, resolved) {
					t.Errorf("want resolved: %+v; got: %+v", want, resolved)
				}
			})
		}
	})

	t.Run("reports an attribute whose location is not four digits as unrecognized", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			entry string
		}{
			{
				name:  "when the location is the global X",
				entry: "X_TW_ATTR_PG_ADMIN",
			},
			{
				name:  "when the location is three digits",
				entry: "123_TW_ATTR_CCE",
			},
			{
				name:  "when the location is five digits",
				entry: "12345_TW_ATTR_CCE",
			},
			{
				name:  "when the location is letters",
				entry: "ABCD_TW_ATTR_CCE",
			},
			{
				name:  "when the location is four digits mixed with a letter",
				entry: "123A_TW_ATTR_CCE",
			},
			{
				name:  "when the location is full-width digits",
				entry: "\uFF11\uFF12\uFF13\uFF14_TW_ATTR_CCE",
			},
			{
				name:  "when the location is four bytes long with a non-ASCII digit",
				entry: "12\u0663_TW_ATTR_CCE",
			},
			{
				name:  "when the location is missing",
				entry: "_TW_ATTR_CCE",
			},
			{
				name:  "when an extra segment sits ahead of the app code",
				entry: "1234_5_TW_ATTR_CCE",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", test.entry}, AppCodeTW)

				want := Groups{
					Roles:        []Role{{Location: "1234", Code: "ROLE_TEACHER"}},
					Attributes:   []string{},
					Unrecognized: []string{test.entry},
				}
				if !reflect.DeepEqual(want, resolved) {
					t.Errorf("want resolved: %+v; got: %+v", want, resolved)
				}
			})
		}
	})

	t.Run("reports an attribute at another school's location as unrecognized without affecting one at the base role's location", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_CCE", "5678_TW_ATTR_SDE"}, AppCodeTW)

		if want, got := []string{"ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{"5678_TW_ATTR_SDE"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("reports an attribute with an unrecognized code as unrecognized without affecting a valid one", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ROLE_TEACHER", "1234_TW_ATTR_CCE", "1234_TW_ATTR_NOPE"}, AppCodeTW)

		if want, got := []string{"ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{"1234_TW_ATTR_NOPE"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("matches an attribute against the base role's location even when the attribute arrives first", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_ATTR_CCE", "1234_TW_ROLE_TEACHER"}, AppCodeTW)

		if want, got := []string{"ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("keeps an attribute granted more than once at the same location once", func(t *testing.T) {
		resolved := ResolveGroups([]string{
			"1234_TW_ROLE_TEACHER",
			"1234_TW_ATTR_CCE",
			"1234_TW_ATTR_SDE",
			"1234_TW_ATTR_CCE",
		}, AppCodeTW)

		if want, got := []string{"ATTR_CCE", "ATTR_SDE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("keeps a base role granted more than once at the same location once", func(t *testing.T) {
		resolved := ResolveGroups([]string{
			"1234_TW_ROLE_TEACHER",
			"1234_TW_ATTR_CCE",
			"1234_TW_ROLE_TEACHER",
		}, AppCodeTW)

		if want, got := []Role{{Location: "1234", Code: "ROLE_TEACHER"}}, resolved.Roles; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Roles: %+v; got: %+v", want, got)
		}
		if want, got := []string{"ATTR_CCE"}, resolved.Attributes; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
		}
		if want, got := []string{}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("reports every attribute as unrecognized", func(t *testing.T) {
		for _, test := range []struct {
			name             string
			raw              []string
			wantUnrecognized []string
		}{
			{
				name:             "when there is no base role",
				raw:              []string{"1234_TW_ATTR_CCE"},
				wantUnrecognized: []string{"1234_TW_ATTR_CCE"},
			},
			{
				name:             "when the only base role is at an MK school",
				raw:              []string{"6101_TW_ROLE_TEACHER", "6101_TW_ATTR_CCE"},
				wantUnrecognized: []string{"6101_TW_ROLE_TEACHER", "6101_TW_ATTR_CCE"},
			},
			{
				name:             "when there is more than one base role",
				raw:              []string{"1234_TW_ROLE_TEACHER", "1234_TW_ROLE_HOD", "1234_TW_ATTR_CCE"},
				wantUnrecognized: []string{"1234_TW_ATTR_CCE"},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups(test.raw, AppCodeTW)

				if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
					t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
				}
				if want, got := test.wantUnrecognized, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
					t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("reports an MK removal when a recognized base role is at an MK school", func(t *testing.T) {
		resolved := ResolveGroups([]string{"6101_TW_ROLE_TEACHER"}, AppCodeTW)

		if !resolved.RemovedMKRole {
			t.Error("want resolved.RemovedMKRole: true; got: false")
		}
	})

	t.Run("does not report an MK removal", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			groups []string
		}{
			{
				name:   "when no entry is at an MK school",
				groups: []string{"1234_TW_ROLE_TEACHER"},
			},
			{
				name:   "when the MK entry carries another application's app code",
				groups: []string{"6101_XX_ROLE_TEACHER"},
			},
			{
				name:   "when the MK entry's role code is unrecognized",
				groups: []string{"6101_TW_ROLE_SUPERINTENDENT"},
			},
			{
				name:   "when the MK entry is an attribute",
				groups: []string{"6101_TW_ATTR_CCE"},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups(test.groups, AppCodeTW)

				if resolved.RemovedMKRole {
					t.Error("want resolved.RemovedMKRole: false; got: true")
				}
			})
		}
	})

	t.Run("reports an entry matching neither infix as unrecognized", func(t *testing.T) {
		resolved := ResolveGroups([]string{"1234_TW_SOMETHING_ELSE"}, AppCodeTW)

		if want, got := []string{"1234_TW_SOMETHING_ELSE"}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
			t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
		}
	})

	t.Run("resolves an empty claim to an empty result", func(t *testing.T) {
		for _, test := range []struct {
			name string
			raw  []string
		}{
			{
				name: "when it is nil",
				raw:  nil,
			},
			{
				name: "when it is an empty list",
				raw:  []string{},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups(test.raw, AppCodeTW)

				want := Groups{
					Roles:        []Role{},
					Attributes:   []string{},
					Unrecognized: []string{},
				}
				if !reflect.DeepEqual(want, resolved) {
					t.Errorf("want resolved: %+v; got: %+v", want, resolved)
				}
			})
		}
	})

	t.Run("rejects an entry from a different application", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			entry string
		}{
			{
				name:  "when it is role-shaped",
				entry: "1234_OTHERAPP_ROLE_TEACHER",
			},
			{
				name:  "when it is attribute-shaped",
				entry: "1234_OTHERAPP_ATTR_CCE",
			},
			{
				name:  "when its prefix merely ends in TW without the underscore",
				entry: "1234_OTHERTW_ROLE_TEACHER",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups([]string{test.entry}, AppCodeTW)

				want := Groups{
					Roles:        []Role{},
					Attributes:   []string{},
					Unrecognized: []string{test.entry},
				}
				if !reflect.DeepEqual(want, resolved) {
					t.Errorf("want resolved: %+v; got: %+v", want, resolved)
				}
			})
		}
	})

	t.Run("rejects an entry from the other Edupass environment", func(t *testing.T) {
		for _, test := range []struct {
			name    string
			appCode AppCode
			entry   string
		}{
			{
				name:    "when the app code is the production one",
				appCode: AppCodeTW,
				entry:   "1234_TWSTG_ROLE_TEACHER",
			},
			{
				name:    "when the app code is the pre-prod one",
				appCode: AppCodeTWSTG,
				entry:   "1234_TW_ROLE_TEACHER",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups([]string{test.entry}, test.appCode)

				want := Groups{
					Roles:        []Role{},
					Attributes:   []string{},
					Unrecognized: []string{test.entry},
				}
				if !reflect.DeepEqual(want, resolved) {
					t.Errorf("want resolved: %+v; got: %+v", want, resolved)
				}
			})
		}
	})

	t.Run("rejects an attribute from the other Edupass environment", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			appCode   AppCode
			baseRole  string
			attribute string
		}{
			{
				name:      "when the app code is the production one",
				appCode:   AppCodeTW,
				baseRole:  "1234_TW_ROLE_TEACHER",
				attribute: "1234_TWSTG_ATTR_CCE",
			},
			{
				name:      "when the app code is the pre-prod one",
				appCode:   AppCodeTWSTG,
				baseRole:  "1234_TWSTG_ROLE_TEACHER",
				attribute: "1234_TW_ATTR_CCE",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolved := ResolveGroups([]string{test.baseRole, test.attribute}, test.appCode)

				if want, got := []string{}, resolved.Attributes; !reflect.DeepEqual(want, got) {
					t.Errorf("want resolved.Attributes: %q; got: %q", want, got)
				}
				if want, got := []string{test.attribute}, resolved.Unrecognized; !reflect.DeepEqual(want, got) {
					t.Errorf("want resolved.Unrecognized: %q; got: %q", want, got)
				}
			})
		}
	})
}

func TestRoleLabel(t *testing.T) {
	t.Run("returns the name Edupass registers", func(t *testing.T) {
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
				label := RoleLabel(test.code)

				if test.want != label {
					t.Errorf("want label: %q; got: %q", test.want, label)
				}
			})
		}
	})

	t.Run("labels every recognized base role code", func(t *testing.T) {
		for code := range roleLabels {
			label := RoleLabel(code)

			if label == "" || label == code {
				t.Errorf("want RoleLabel(%q): a label; got: %q", code, label)
			}
		}
	})

	t.Run("returns an empty label when the code is unrecognized", func(t *testing.T) {
		label := RoleLabel("ROLE_UNKNOWN")

		if label != "" {
			t.Errorf("want label: empty; got: %q", label)
		}
	})
}
