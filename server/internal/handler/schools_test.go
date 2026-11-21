package handler

import (
	"testing"

	"github.com/transformteamsg/teacher-workspace/server/internal/config"
)

func TestSchoolNamesFor(t *testing.T) {
	t.Run("returns real school names for the production app code", func(t *testing.T) {
		names := schoolNamesFor(config.EdupassAppCodeTW)

		name, ok := names["3015"]
		if !ok {
			t.Fatal("want names[\"3015\"] ok: true; got: false")
		}
		if want := "Whitley Secondary School"; want != name {
			t.Errorf("want names[\"3015\"]: %q; got: %q", want, name)
		}
		if want, got := "Staging School 1001", names["1001"]; want == got {
			t.Errorf("want names[\"1001\"]: != %q; got: %q", want, got)
		}
	})

	t.Run("returns made-up school names for the pre-prod app code", func(t *testing.T) {
		names := schoolNamesFor(config.EdupassAppCodeTWSTG)

		name, ok := names["1001"]
		if !ok {
			t.Fatal("want names[\"1001\"] ok: true; got: false")
		}
		if want := "Staging School 1001"; want != name {
			t.Errorf("want names[\"1001\"]: %q; got: %q", want, name)
		}
		if want, got := "Whitley Secondary School", names["3015"]; want == got {
			t.Errorf("want names[\"3015\"]: != %q; got: %q", want, got)
		}
	})

	t.Run("returns no school names for an unrecognised app code", func(t *testing.T) {
		names := schoolNamesFor("XX")

		if names != nil {
			t.Errorf("want names: nil; got: %v", names)
		}
	})
}
