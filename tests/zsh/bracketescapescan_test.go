// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A backslash inside a bracket expression is invisible to nothing.
//
// The scans that walk a pattern looking for where a bracket ends — interp's
// bracketEnd, and skipBracket over it — read the `]` of a `\]` as the one
// that closes the expression, where the matcher and glob.go's closesBracket
// both read it as a protected member. Two helpers answering the same
// question two ways, which is the whole of #4212: `[\]~#]` was cut at a `~`
// the walker believed stood at top level, and the `#` left behind it had
// nothing closable in front of it, so a pattern real zsh matches happily was
// called `bad pattern` instead.
//
// zsh-autosuggestions writes that bracket on every keystroke —
// `${1//(#m)[\\*?[\]<>()|^~#]/\\$MATCH}` at zsh-autosuggestions.zsh:652 — so
// the fault printed a diagnostic per character typed at a real prompt.
//
// Measured against zsh 5.9.2 on 2026-09-22.
func TestABackslashInABracketDoesNotEndIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The two rows from the report. The first refused the pattern; the
		// second was silently wrong, the closure binding to a bracket cut
		// one byte short of its end.
		{`setopt extendedglob; x=a; print -r -- "${x//[\]~#]/X}"`, "a"},
		{`setopt extendedglob; x=a; print -r -- "${x//[\]]#/X}"`, "Xa"},
		// The plugin's own pattern, whole.
		{`setopt extendedglob; x="a*b"; print -r -- "${x//(#m)[\\*?[\]<>()|^~#]/\\$MATCH}"`, `a\*b`},
		// The bracket still holds what it held: the escaped `]` is a member,
		// and a member beside it is unharmed.
		{`setopt extendedglob; x="]"; print -r -- "${x//[\]]/X}"`, "X"},
		{`setopt extendedglob; x=a; print -r -- "${x//[\]a]/X}"`, "X"},
		{`setopt extendedglob; x=b; print -r -- "${x//[\]a]/X}"`, "b"},
		// A `~` outside a bracket is still an exclusion, which is the half
		// of the scan this must not have bought its fix from. `Xb` and not
		// `ab`: a global replacement re-applies, so at the front the longest
		// match holding no `b` is the `a` alone, and the `b` is left where
		// it stood. Measured, and this row is here because the first guess
		// at it was `ab` and real zsh said otherwise.
		{`setopt extendedglob; print -r -- ${${:-ab}//a*~*b*/X}`, "Xb"},
		{`setopt extendedglob; print -r -- ${${:-ac}//a*~*b*/X}`, "X"},
		// And #1409's rows, which are why bracketEnd steps over a
		// `[:class:]` whole: a closure over a class still finds its class.
		{`setopt extendedglob; v="  x"; print -r -- "${v##[[:space:]]##}"`, "x"},
		{`setopt extendedglob; v=abX; print -r -- "${v##[[:alpha:]]##}"`, ""},
		// With the option off the `#` is ordinary text again, so the bracket
		// is the whole pattern and nothing in it matches an `a`.
		{`x=a; print -r -- "${x//[\]~#]/X}"`, "a"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}
