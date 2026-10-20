// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A listing spells an alias's **name** the way it spells the value, where the
// name needs it. This shell is the only one in the panel that does.
//
// Measured on zsh 5.9.2, 2026-09-12 and re-measured 2026-09-13, `env -i
// PATH=/usr/bin:/bin` over a script file. The name rule is the value rule
// character for character — swept over 37 spellings, every byte quoted in a
// value is quoted in a name and every byte left bare in one is left bare in
// the other — so there is nothing here that AliasQuoting does not already
// say, only whether it is asked (#2579).
func TestAListingQuotesANameThatNeedsIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`alias 'a$b'=echo; alias 'a$b'`, "'a$b'=echo\n"},
		{`alias 'a b'=echo; alias 'a b'`, "'a b'=echo\n"},
		{`alias 'a#b'=echo; alias 'a#b'`, "'a#b'=echo\n"},
		// The quote inside a name goes through the same run-splitting the
		// value gets, which is what says one spelling serves both.
		{`alias "a'b"=echo; alias "a'b"`, "'a'\\''b'=echo\n"},
		// A name needing nothing is left alone, so the change is about the
		// names that need it and not about every line of every listing.
		{`alias ab=echo; alias ab`, "ab=echo\n"},
		{`alias a-b.c=echo; alias a-b.c`, "a-b.c=echo\n"},
		// Every listing form that would define the entry back, and the two
		// other namespaces.
		{`alias 'a$b'=echo; alias -L 'a$b'`, "alias 'a$b'=echo\n"},
		{`alias -g 'g$b'=echo; alias -g -L 'g$b'`, "alias -g 'g$b'=echo\n"},
		{`alias -s 'x$y'=cat; alias -s 'x$y'`, "'x$y'=cat\n"},
		{`alias -s 'x$y'=cat; alias -s -L 'x$y'`, "alias -s 'x$y'=cat\n"},
		{`alias 'a$b'=echo; command -v 'a$b'`, "alias 'a$b'=echo\n"},
		// The letter stays outside the quotes, or the line would not define
		// the entry back.
		{`alias -g 'g$b'=echo; command -v 'g$b'`, "alias -g 'g$b'=echo\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the routes it does not reach. A sentence is prose about a name rather
// than a line that would define the entry back, and the names-only listing is
// a column of names rather than a definition — measured the same day, all
// four of these write the name bare.
func TestASentenceAboutAnAliasLeavesTheNameBare(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`alias 'a$b'=echo; type 'a$b'`, "a$b is an alias for echo\n"},
		{`alias 'a$b'=echo; command -V 'a$b'`, "a$b is an alias for echo\n"},
		{`alias 'a$b'=echo; whence -v 'a$b'`, "a$b is an alias for echo\n"},
		{`alias 'a$b'=echo; alias +`, "a$b\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
