// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A substring range has its pattern characters protected before it is read,
// so every one of them is an arithmetic syntax error here.
//
// Measured 2026-09-13 against ksh93u+ 2012-08-01. The issue was filed about
// `${s:(-2)}` and the parentheses are not what it is about: `${s:(2)}` and
// `${s:1+(1)}` refuse with no sign in them, `${s: -2}` is taken with a sign
// and no parentheses, and `${s:1|2}`, `${s:1&3}`, `${s:1*2}` and
// `${s:1?2:3}` refuse while `${s:1<2}`, `${s:1%2}`, `${s:1^2}`, `${s:1,2}`
// and `${s:1/1}` are all evaluated (#2618).
func TestARangesPatternCharactersAreProtected(t *testing.T) {
	for _, c := range []struct{ src, blame string }{
		{`s=hello; printf "[%s]" "${s:(-2)}"`, `\(-2\)`},
		{`s=hello; printf "[%s]" "${s:(2)}"`, `\(2\)`},
		{`s=hello; printf "[%s]" "${s:1+(1)}"`, `1+\(1\)`},
		{`s=hello; printf "[%s]" "${s:1|2}"`, `1\|2`},
		{`s=hello; printf "[%s]" "${s:1&3}"`, `1\&3`},
		{`s=hello; printf "[%s]" "${s:1*2}"`, `1\*2`},
		{`s=hello; printf "[%s]" "${s:1?2:3}"`, `1\?2:3`},
		// The length half, which is protected on its own and blamed on its
		// own: the offset in front of it evaluated and is not named.
		{`s=hello; printf "[%s]" "${s:1:(2)}"`, `\(2\)`},
		// And the offset with a length behind it, where this shell names
		// the whole range — both halves protected, then joined.
		{`s=hello; printf "[%s]" "${s:(1):2}"`, `\(1\):2`},
		{`s=hello; printf "[%s]" "${s:1+:(2)}"`, `1+:\(2\)`},
		// The characters that cannot be written into a range directly,
		// reached through a value, which is also what says the protection
		// happens after the expansion rather than to the source text.
		{`s=hello; q="9]9"; printf "[%s]" "${s:$q}"`, `9\]9`},
		{`s=hello; q="9[9"; printf "[%s]" "${s:$q}"`, `9\[9`},
		{`s=hello; q="9}9"; printf "[%s]" "${s:$q}"`, `9\}9`},
		{`s=hello; q="9\\9"; printf "[%s]" "${s:$q}"`, `9\\9`},
	} {
		out, st := kshOut(t, c.src)
		if st == 0 {
			t.Errorf("%s = %q at status 0, want a refusal", c.src, out)
			continue
		}
		want := c.blame + ": arithmetic syntax error"
		if !strings.Contains(out, want) {
			t.Errorf("%s = %q, want it to name %q", c.src, out, want)
		}
	}
}

// The characters that are *not* protected, which is what makes this a set
// rather than "punctuation in a range is refused". Every one of these is an
// ordinary expression here and answers with a slice.
func TestARangeKeepsTheCharactersTheProtectionDoesNotCover(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`s=hello; printf "[%s]" "${s: -2}"`, `[lo]`},
		{`s=hello; printf "[%s]" "${s:1<2}"`, `[ello]`},
		{`s=hello; printf "[%s]" "${s:1%2}"`, `[ello]`},
		{`s=hello; printf "[%s]" "${s:1^2}"`, `[lo]`},
		{`s=hello; printf "[%s]" "${s:1,2}"`, `[llo]`},
		{`s=hello; printf "[%s]" "${s:1/1}"`, `[ello]`},
		{`s=hello; printf "[%s]" "${s:1:3}"`, `[ell]`},
		{`s=hello; n=2; printf "[%s]" "${s:n}"`, `[llo]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// It is the range and nothing else. The same characters in the same shell's
// other arithmetic sites are evaluated, so this cannot be folded into a rule
// about the arithmetic reader — measured, `$(( (1) ))` is 1 and
// `a=(x y z); $(( a[(1)] ))` reads the second element.
func TestTheProtectionIsTheRangeAndNotTheArithmetic(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`printf "[%s]" "$(( (1) ))"`, `[1]`},
		{`printf "[%s]" "$(( 1+(1) ))"`, `[2]`},
		{`a=(x y z); printf "[%s]" "${a[(1)]}"`, `[y]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The axis rather than the code, so a preset that stopped holding it fails
// here rather than only in a wording comparison.
func TestTheRangeProtectionIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().SubstringRangeQuotesPatternCharacters; got != interp.Yes {
		t.Errorf("SubstringRangeQuotesPatternCharacters = %v, want yes", got)
	}
}
