package session

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	t.Run("returns a value twice the length of the token", func(t *testing.T) {
		masked := mask([]byte("123456"))

		if want, got := 12, len(masked); want != got {
			t.Errorf("want len(masked): %d; got: %d", want, got)
		}
	})

	t.Run("returns a value that unmask reverses to the token", func(t *testing.T) {
		masked := mask([]byte("123456"))

		token, ok := unmask(masked)
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := []byte("123456"); !bytes.Equal(want, token) {
			t.Errorf("want token: %q; got: %q", want, token)
		}
	})

	t.Run("returns a value that differs on every call", func(t *testing.T) {
		first := mask([]byte("123456"))
		second := mask([]byte("123456"))

		if bytes.Equal(first, second) {
			t.Errorf("want second: != %x; got: %x", first, second)
		}
	})
}

func TestUnmask(t *testing.T) {
	t.Run("returns the original token from a value produced by mask", func(t *testing.T) {
		masked := mask([]byte("abcd"))

		token, ok := unmask(masked)
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want := []byte("abcd"); !bytes.Equal(want, token) {
			t.Errorf("want token: %q; got: %q", want, token)
		}
	})

	t.Run("reports false when the value has an odd length", func(t *testing.T) {
		_, ok := unmask([]byte("odd"))
		if ok {
			t.Error("want ok: false; got: true")
		}
	})
}

func TestMaskToken(t *testing.T) {
	t.Run("returns a masked form of the token", func(t *testing.T) {
		token := "1234"

		masked := maskToken(token)
		if !verifyToken(token, masked) {
			t.Error("want verifyToken(token, masked): true; got: false")
		}
	})

	t.Run("returns a URL-safe value", func(t *testing.T) {
		// A long token makes every base64 character likely to appear, so a
		// non-URL-safe encoding can't pass by chance.
		masked := maskToken(strings.Repeat("x", 256))

		if got := url.QueryEscape(masked); masked != got {
			t.Errorf("want url.QueryEscape(masked): %q; got: %q", masked, got)
		}
	})

	t.Run("returns a value that differs on every call", func(t *testing.T) {
		first := maskToken("1234")
		second := maskToken("1234")

		if first == second {
			t.Errorf("want second: != %q; got: %q", first, second)
		}
	})
}

func TestVerifyToken(t *testing.T) {
	t.Run("reports whether the value is a masked form of the token", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			token  string
			masked string
			want   bool
		}{
			{
				name:   "true for a value returned by maskToken",
				token:  "abcd",
				masked: maskToken("abcd"),
				want:   true,
			},
			{
				name:   "false when the token is empty",
				token:  "",
				masked: "",
				want:   false,
			},
			{
				name:   "false for a value masked from another token",
				token:  "abcd",
				masked: maskToken("wxyz"),
				want:   false,
			},
			{
				name:   "false for the unmasked token",
				token:  "abcd",
				masked: "abcd",
				want:   false,
			},
			{
				name:   "false for a value that is not base64",
				token:  "abcd",
				masked: "!!!!",
				want:   false,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				verified := verifyToken(test.token, test.masked)
				if want := test.want; want != verified {
					t.Errorf("want verified: %t; got: %t", want, verified)
				}
			})
		}
	})
}
