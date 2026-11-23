// Package edupass holds checks on values issued by Edupass.
package edupass

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
