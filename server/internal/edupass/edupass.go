// Package edupass interprets values issued by Edupass.
package edupass

// AppCode is Teacher Workspace's Edupass app code: the <app_code> in each App
// Role ID Edupass issues it, named `<location_code>_<app_code>_<role_code>`,
// such as "TW" in `1234_TW_ROLE_TEACHER`. Production Edupass issues
// [AppCodeTW], and pre-prod Edupass [AppCodeTWSTG].
type AppCode string

const (
	AppCodeTW    AppCode = "TW"
	AppCodeTWSTG AppCode = "TWSTG"
)

// IsLocationCode reports whether code is a school's location code: exactly
// four ASCII digits.
func IsLocationCode(code string) bool {
	if len(code) != 4 {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}
