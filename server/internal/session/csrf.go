package session

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// mask returns a masked form of token, twice its length, that differs on every
// call. unmask reverses it.
func mask(token []byte) []byte {
	n := len(token)
	masked := make([]byte, 2*n)

	otp, encrypted := masked[:n], masked[n:]
	_, _ = rand.Read(otp)
	subtle.XORBytes(encrypted, otp, token)
	return masked
}

// unmask returns the original token from a value produced by mask. It reports
// false when token has an odd length.
func unmask(token []byte) ([]byte, bool) {
	if len(token)%2 != 0 {
		return nil, false
	}

	n := len(token) / 2
	otp, encrypted := token[:n], token[n:]
	unmasked := make([]byte, n)
	subtle.XORBytes(unmasked, otp, encrypted)
	return unmasked, true
}

// maskToken returns a URL-safe masked form of token that differs on every call.
func maskToken(token string) string {
	return base64.RawURLEncoding.EncodeToString(mask([]byte(token)))
}

// verifyToken reports whether masked is a masked form of token, such as one
// returned by maskToken. It compares in constant time, so its timing reveals
// nothing about token beyond its length. An empty token never verifies.
func verifyToken(token, masked string) bool {
	n := len(token)
	if n == 0 {
		return false
	}

	b, err := base64.RawURLEncoding.DecodeString(masked)
	if err != nil || len(b) != 2*n {
		return false
	}

	got, _ := unmask(b)
	return subtle.ConstantTimeCompare([]byte(token), got) == 1
}
