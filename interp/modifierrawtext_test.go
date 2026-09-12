// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A modifier reads its own **text**, and that text is the source between the
// braces — no quote character taken off it, and no expansion performed in it
// (#1860).
//
// It is the only operand of a `${ }` that reads text rather than a value,
// which is what makes this visible here and nowhere else: the same missing
// byte changes nothing for an operand that is a value by the time anything
// looks at it.
//
// Measured on zsh 5.9.2, 2026-09-12, from a script file so the quoting around
// the expansion is not what is being tested.
func TestAModifierReadsTheTextAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The two rows the issue was filed on, and they are each other's
		// control: the pattern the quotes are part of matches the value that
		// holds them and misses the one that does not.
		{"a quoted pattern matches the quotes", `x="a'.'b"; printf "[%s]" "${x:s/'.'/:/}"`, "[a:b]"},
		{"and misses the value without them", `y='a.b'; printf "[%s]" "${y:s/'.'/:/}"`, "[a.b]"},
		// The same claim with no metacharacter anywhere in it, so nothing
		// about patterns can be what is being measured.
		{"with no metacharacter in sight", `w='aQb'; printf "[%s]" "${w:s/'Q'/X/}"`, "[aQb]"},
		{"a quote is one character of the pattern", `u="a'b"; printf "[%s]" "${u:s/\'/X/}"`, "[aXb]"},
		{"and blanks inside quotes are blanks", `v='a  b'; printf "[%s]" "${v:s/'  '/_/}"`, "[a  b]"},
		// The sharper half: the text is not *expanded* either, so a word
		// joined from spans is wrong in a second way. Row one would answer
		// `aQb` and row two `aQb` under a reading that expanded it.
		{"a dollar is text", `a=X; x=aXb; printf "[%s]" "${x:s/$a/Q/}"`, "[aXb]"},
		{"and so is one inside quotes", `a=X; z='a$ab'; printf "[%s]" "${z:s/'$a'/Q/}"`, "[a$ab]"},
		// The backslash the same text already carried (#1198), unmoved.
		{"a backslash still protects the delimiter", `p='a/b'; printf "[%s]" "${p:s/\//:/}"`, "[a:b]"},
		{"and still escapes a dot into itself", `y='a.b'; printf "[%s]" "${y:s/\./:/}"`, "[a:b]"},
		// A modifier behind a substring reads its own text the same way, and
		// that is the second call site.
		{"a modifier behind an offset", `q='xxa/b'; printf "[%s]" "${q:2:s/'\/'/-/}"`, "[a/b]"},
		{"and a quoted dot there misses too", `q='xxa.b'; printf "[%s]" "${q:2:s/'.'/-/}"`, "[a.b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWithModifiers(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
