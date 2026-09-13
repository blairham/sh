// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// keyBracketRun runs one snippet with the grammar an associative subscript
// needs and one answer for the axis under test.
//
// Everything else is held still: the arrays count from zero and a plain name
// is not the whole array, so the only thing that can move a row is whether
// the subscript's expanded text is read again as syntax.
func keyBracketRun(t *testing.T, src string, answer Answer) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		r.Semantics.ArithSubscriptRereadsItsExpandedText = answer
		// The re-reading answer walks into an empty subscript — `b[` with
		// the key's own `]` behind it — so the axis for *that* has to be
		// held too or the row is refused for the wrong reason.
		r.Semantics.EmptyArithSubscript = EmptyArithSubscriptIsInvalid
	})
}

// An associative key is a string, and one reading hands it back to the
// bracket scanner once it has been expanded.
//
// Under the protecting answer the element is found and incremented; under the
// re-reading one the key's own `]` closes the subscript, the `b[` behind it
// is a second name with an empty subscript, and the element is left alone.
// The pair is the axis (#2581).
func TestAnExpandedSubscriptIsReadAgainOrNot(t *testing.T) {
	const src = `typeset -A m; key='x],b['; m[$key]=1; (( m[$key]++ )); printf "[%s]" "${m[$key]}"`
	if out, st := keyBracketRun(t, src, No); out != `[2]` || st != 0 {
		t.Errorf("protecting = %q status %d, want %q at 0", out, st, `[2]`)
	}
	// Under the other answer the key's own `]` closes the subscript, the
	// `b[` behind it is a second name with an empty one, and the element is
	// never touched. The complaint and the untouched element together,
	// because either alone would pass against a reading that answered
	// quietly or one that complained and wrote anyway. What *status* the
	// complaint carries is the dialect's question and is pinned in
	// dialect/zsh.
	out, _ := keyBracketRun(t, src, Yes)
	if !strings.Contains(out, "subscript") {
		t.Errorf("re-reading = %q, want a complaint about the subscript", out)
	}
	if !strings.HasSuffix(out, `[1]`) {
		t.Errorf("re-reading = %q, want the element left at 1", out)
	}
}

// A key holding the other two characters a subscript scanner reads, and the
// comma that made the expanded text a list of two expressions.
func TestAProtectedSubscriptKeepsEveryCharacterOfTheKey(t *testing.T) {
	for _, c := range []struct{ key, want string }{
		{`x],b[`, `[2]`},
		{`a,b`, `[2]`},
		{`a[b`, `[2]`},
		{`a]b`, `[2]`},
	} {
		src := `typeset -A m; key='` + c.key + `'; m[$key]=1; (( m[$key]++ )); printf "[%s]" "${m[$key]}"`
		if out, st := keyBracketRun(t, src, No); out != c.want || st != 0 {
			t.Errorf("key %q = %q status %d, want %q at 0", c.key, out, st, c.want)
		}
	}
}

// The protection is about where in the *source* the expansion stands, not
// about it being an expansion. Both rows below hold under either answer,
// which is what says the axis was asked in the right place.
func TestAnExpansionOutsideBracketsIsStillReadAsSyntax(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		// No bracket of the script's anywhere, so the value's brackets are
		// the subscript.
		if out, st := keyBracketRun(t, `a=(9 8 7); v="a[1]"; printf "[%s]" "$(( $v ))"`, answer); out != `[8]` || st != 0 {
			t.Errorf("with %v = %q status %d, want %q at 0", answer, out, st, `[8]`)
		}
		// And the script left its own bracket open, so there is none of its
		// own for a value's to be distinguished from.
		if out, st := keyBracketRun(t, `a=(9 8 7); k="1]"; printf "[%s]" "$(( a[$k ))"`, answer); out != `[8]` || st != 0 {
			t.Errorf("unclosed with %v = %q status %d, want %q at 0", answer, out, st, `[8]`)
		}
	}
}

// The expression around the subscript is still expanded and read as one, so
// the protection has not turned the whole text into data.
func TestTheExpressionAroundAProtectedSubscriptStillReads(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x="1+"; y="2"; printf "[%s]" "$(( $x$y ))"`, `[3]`},
		{`a=(9 8 7); i=2; printf "[%s]" "$(( a[$i] + 1 ))"`, `[8]`},
		{`typeset -A m; m[k]=3; printf "[%s]" "$(( m[k] + m[k] ))"`, `[6]`},
		// Two subscripts in one expression, each with its own expansion.
		{`typeset -A m; p=a; q=b; m[$p]=1; m[$q]=2; printf "[%s]" "$(( m[$p] + m[$q] ))"`, `[3]`},
	} {
		if out, st := keyBracketRun(t, c.src, No); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// A subscript's expansion is performed once. The increment reads and writes
// through the same brackets, and a command substitution in the key must not
// run a second time from the arithmetic evaluator — measured, no column in
// the panel does.
func TestAProtectedSubscriptExpandsItsKeyOnce(t *testing.T) {
	src := `typeset -A m; key="k$(printf RAN)"; m[$key]=1; (( m[$key]++ )); printf "[%s]" "${m[kRAN]}"`
	if out, st := keyBracketRun(t, src, No); out != `[2]` || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, `[2]`)
	}
}
