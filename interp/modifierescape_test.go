// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A modifier reads its own text rather than a value, so a backslash written
// inside `${ }` has to survive as far as the modifier.
//
// It did not: the joined text of a word is its spans with their delimiters
// taken off, which is exactly right for an operand that is a value and loses
// the one byte that decides where a substitution's fields end. `${x:s/\//:/}`
// arrived as `s///` — an empty pattern, which is this modifier's spelling for
// "the previous substitution" — and so reported there had not been one
// (#1198). The protected character is a span of its own, so the backslash is
// written back in front of it.
//
// docs/spec/semantics.md, the `:s` modifier.
func TestABackslashReachesTheModifierThatReadsIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The shape the fault was found in: the value's own separator is
		// also the delimiter, so the escape is the only thing holding the
		// field open.
		{"an escaped delimiter does not end the field", `x=a/b/c; echo "[${x:s/\//:/}]"`, "[a:b/c]\n"},
		{"and the global spelling takes them all", `x=a/b/c; echo "[${x:gs/\//:/}]"`, "[a:b:c]\n"},
		{"the escaped delimiter may be the replacement", `x=a:b; echo "[${x:s/:/\//}]"`, "[a/b]\n"},
		// The backslash goes whatever it stood before, so an escape is never
		// left in the string it was protecting a byte of.
		{"a backslash before an ordinary byte is removed", `x=a.b; echo "[${x:s/\./:/}]"`, "[a:b]\n"},
		{"a doubled backslash is one literal backslash", `x='a\ab'; echo "[${x:s/\\a/:/}]"`, "[a:b]\n"},
		// The replacement asks one more question of its escapes than the
		// pattern does, and the order of the two is observable.
		{"an escaped ampersand is the character", `x=aXbXc; echo "[${x:s/X/[\&]/}]"`, "[a[&]bXc]\n"},
		{"a bare ampersand is still the matched text", `x=aXbXc; echo "[${x:s/X/[&]/}]"`, "[a[X]bXc]\n"},
		{"a backslash then the matched text", `x=aXbXc; echo "[${x:s/X/[\\&]/}]"`, "[a[\\X]bXc]\n"},
		// Unquoted is the same reading: nothing here is a double-quote rule.
		{"unquoted", `x=a/b/c; printf '[%s]\n' ${x:s/\//:/}`, "[a:b/c]\n"},
		// The escapes are the modifier's own and stop at its edge: a letter
		// that takes no argument is unmoved, and so is the next modifier.
		{"a chained modifier after a substitution", `x=a/b/c; echo "[${x:s/\//:/:u}]"`, "[A:B/C]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWithModifiers(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// What is remembered is the pattern as the modifier read it, not as it was
// written: an empty pattern reaches for the string that was searched for,
// which is the escape's result and not the escape.
func TestTheRememberedPatternIsTheOneThatWasSearchedFor(t *testing.T) {
	const src = `x=a/b/c; y=d/e; echo "[${x:s/\//:/}][${y:s//+/}]"`
	out, st := runWithModifiers(t, src)
	if want := "[a:b/c][d+e]\n"; out != want || st != 0 {
		t.Errorf("%s\ngot  %q (status %d)\nwant %q", src, out, st, want)
	}
}
