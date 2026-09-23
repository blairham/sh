// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// runBig5Units runs src under a Big5 locale with the encoding axis answered.
//
// A separate helper from runBig5 beside it because it is a separate axis:
// that one answers what reaches `read`, this one whether a character is the
// locale's at all, and a length asks only the second.
func runBig5Units(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := *r.Semantics
		sem.MultibyteEncodingIsHonored = answer
		r.Semantics = &sem
		r.Env = append(append([]string(nil), r.Env...), "LC_ALL=zh_TW.Big5")
	})
}

// A length and a position count the locale's characters where the locale names
// a charset this implementation can measure, which Big5 and Shift-JIS are.
//
// Measured 2026-09-23 under `LC_ALL=zh_TW.Big5` on bash 5.3.15, bash 3.2.57,
// ksh93u+ and zsh 5.9.2: all four answer 2 for a value of U+03B1 followed by
// `Z`, and all four give the whole two-byte character for `${v:0:1}`. dash
// counts bytes there as it counts bytes everywhere, which is the split
// MultibyteEncodingIsHonored already records — so this is a second encoding
// under the existing axis rather than an axis of its own.
func TestALengthCountsTheLocalesCharacters(t *testing.T) {
	// Single-quoted throughout, because the trail byte of this character is
	// a backslash and an unquoted one would quote the letter behind it.
	const src = `v='` + big5Alpha + `Z'; printf '[%s][%s]' "${#v}" "${v:0:1}"`
	for _, c := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"the locale's characters", Yes, "[2][" + big5Alpha + "]"},
		// Byte by byte the trail byte is a character of its own, so the
		// length is one more and the first position is half a letter.
		{"bytes", No, "[3][\xa3]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBig5Units(t, c.answer, src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// A separator of IFS is a whole character, and the walk moves past all of it.
//
// The two shapes are one rule and they reach it by different routes: an
// unquoted expansion splits the *escaped* form, where the value's own
// backslash — which is this character's trail byte — carries a mark, and
// `read` splits plain text. Told only where a separator begins, the walk
// stepped one byte past it and left the backslash half of the letter on the
// front of the next field.
func TestASeparatorIsAWholeCharacterOfTheLocale(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{
			"an unquoted expansion",
			`v='X` + big5Alpha + `Y'; IFS='` + big5Alpha + `'; set -- $v; printf '[%d][%s][%s]' "$#" "$1" "$2"`,
			"[2][X][Y]",
		},
		{
			"read",
			`IFS='` + big5Alpha + `'; read a b <<'EOF'` + "\nX" + big5Alpha + "Y\nEOF\n" +
				`printf '[%s][%s]' "$a" "$b"`,
			"[X][Y]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBig5Units(t, Yes, c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}
