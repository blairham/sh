// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `a b () { … }` — the parenthesis spelling of one body under several names,
// run rather than only parsed.
//
// The same promise the keyword spelling makes: `$0` inside the body is the
// name that was *called*, so a reading that defined one function and pointed
// the rest at it would parse and answer every call with the same name (#1685).
func TestTheParenthesisSpellingDefinesEveryNameToo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a b () { echo "$0"; }; a; b`, "a\nb"},
		{`clipcopy clippaste() { echo "$0"; }; clipcopy; clippaste`, "clipcopy\nclippaste"},
		{`a b c () { echo "$0"; }; c; a; b`, "c\na\nb"},
		// Any word list, so a word that is a command name everywhere else is
		// a name here — this redefines `echo` and `hi` both.
		{`echo hi () { print -r -- "[$0]"; }; hi`, "[hi]"},
		// The arguments are the call's own.
		{`a b () { echo "$0 $#"; }; a x; b x y z`, "a 1\nb 3"},
		// A quoted name is a name whatever is in it, in the list too.
		{`a "b c" () { echo "[$0]"; }; "b c"`, "[b c]"},
		// A later name stands where an argument stands, so an `=` in it is
		// ordinary text rather than the assignment that ends a first name.
		{`a c=d () { echo "$0"; }; print -rl -- ${(ko)functions}`, "a\nc=d"},
		// A name may hold an expansion here, in the list as in front of it.
		{`w=x; a p_$w () { echo "$0"; }; p_x`, "p_x"},
		// And one name is still one name, which is the control.
		{`a () { echo "$0"; }; a`, "a"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
