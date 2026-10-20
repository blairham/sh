// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// An associative key is a string here, and a subscript that has just been
// expanded is not handed back to the bracket scanner.
//
// Measured 2026-09-13 against bash 5.3.15 and under `sh`: with `key='x],b['`
// the element is stored, incremented and read back, at status 0 — where
// reading the expanded `m[x],b[++` again as syntax finds a subscript of `x`,
// a comma, and a second name with an empty subscript. zsh is the column that
// does read it again (#2581).
func TestAKeyHoldingABracketSurvivesTheArithmetic(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`declare -A m; key='x],b['; m[$key]=1; (( m[$key]++ )); printf "[%s][%s]" "$?" "${m[$key]}"`, `[0][2]`},
		{`declare -A m; key='a,b'; m[$key]=1; (( m[$key]++ )); printf "[%s]" "${m[$key]}"`, `[2]`},
		{`declare -A m; key='a[b'; m[$key]=1; (( m[$key]++ )); printf "[%s]" "${m[$key]}"`, `[2]`},
	} {
		if out, st := answersRun(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The two boundaries, which is what says the protection is about where in the
// *source* the expansion stands rather than about it being an expansion.
func TestABracketFromAValueStillDelimitsWhereTheSourceWroteNone(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The script left its own bracket open, so the value's closes it.
		{`a=(9 8 7); k="1]"; printf "[%s]" "$(( a[$k ))"`, `[8]`},
		// And with no bracket of the script's anywhere, the value's are the
		// subscript.
		{`a=(9 8 7); v="a[1]"; printf "[%s]" "$(( $v ))"`, `[8]`},
		// The expression around a protected subscript is still an
		// expression, built out of expansions like any other.
		{`x="1+"; y="2"; printf "[%s]" "$(( $x$y ))"`, `[3]`},
	} {
		if out, st := answersRun(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// A subscript whose text is not an expression is still blamed as the text it
// came to, so the protection has not swallowed the refusal.
func TestAProtectedSubscriptThatIsNoExpressionIsStillBlamed(t *testing.T) {
	out, st := answersRun(t, `a=(9 8 7); k="1]+a[2"; printf "[%s]" "$(( a[$k] ))"`)
	if st == 0 {
		t.Fatalf("= %q at status 0, want a refusal", out)
	}
	if want := "1]+a[2"; !strings.Contains(out, want) {
		t.Errorf("= %q, want it to name %q", out, want)
	}
}

// The axis, and it is the answer the standard's base already holds — so this
// pins that no preset here has moved off it.
func TestTheProtectionIsAnAxis(t *testing.T) {
	if got := bash.Semantics().ArithSubscriptRereadsItsExpandedText; got != interp.No {
		t.Errorf("ArithSubscriptRereadsItsExpandedText = %v, want no", got)
	}
}
