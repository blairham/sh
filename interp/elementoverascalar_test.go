// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript on the left of `=` turns a name holding a string into an array,
// and the string it was holding is the first element of the one it becomes.
//
// It was dropped. `a=abc; a[1]=x` left one element at subscript 1 — the right
// shape, at status 0, with the script's own value silently gone out of it
// (#1570). The same fault #1502 fixed for `a+=(x)`, one layer along: there
// the operator made the name an array and here the subscript does.
func TestAnElementWrittenOverAScalarKeepsIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the value becomes the first element", `a=abc; a[1]=x`, "[abc][x] keys=0 1 n=2"},
		{"however far past it the element lands", `a=abc; a[2]=x`, "[abc][x] keys=0 2 n=2"},
		{"and the first element is written over", `a=abc; a[0]=x`, "[x] keys=0 n=1"},
		// The append spelling reads the promotion as well as making it:
		// there is something to join to at the base, which is where the
		// value that was dropped used to leave nothing.
		{"an append at the base joins the value", `a=abc; a[0]+=x`, "[abcx] keys=0 n=1"},
		{"an append past it has nothing to join", `a=abc; a[1]+=x`, "[abc][x] keys=0 1 n=2"},
		// An empty value is a value and an unset name is not, which is the
		// distinction the store already draws for `a+=(x)`.
		{"an empty value is still a value", `a=; a[1]=x`, "[][x] keys=0 1 n=2"},
		{"an unset name has nothing to keep", `unset a; a[1]=x`, "[x] keys=1 n=1"},
		// The controls: a name already holding an array is untouched by any
		// of this, and the whole-array spelling replaces rather than keeps.
		{"an array is written, not promoted", `a=(p q); a[1]=x`, "[p][x] keys=0 1 n=2"},
		{"an array append is unmoved", `a=(p q); a[0]+=x`, "[px][q] keys=0 1 n=2"},
		{"the whole-array spelling replaces", `a=abc; a=(x)`, "[x] keys=0 n=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + `; printf "[%s]" "${a[@]}"; echo " keys=${!a[@]} n=${#a[@]}"`
			out, st := runArray(t, src)
			if got := strings.TrimSpace(out); got != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// The name's attributes fold the join once, which is what says the promoted
// element is an element and not a string put back by hand.
func TestAnAppendOverAPromotedScalarFoldsTheAttribute(t *testing.T) {
	out, st := runArray(t, `typeset -i a=5; a[0]+=3; printf "[%s]" "${a[@]}"`)
	if out != "[8]" || st != 0 {
		t.Errorf("= %q (status %d), want [8] at 0", out, st)
	}
}

// *That* the value is kept is core. *When* it is kept relative to reading the
// subscript is not, and a subscript counting back from the end is the only
// spelling that can tell the two apart.
//
// See Semantics.NegativeSubscriptCountsOverAPromotedScalar.
func TestANegativeSubscriptOverAPromotedScalarAsksTheAxis(t *testing.T) {
	const src = `a=abc; a[-1]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
	// Counting back over the element the promotion made: one element, and the
	// value it was holding is what was written over.
	out, st := runPromotedNegative(t, Yes, src)
	if want := "[x] n=1\n"; out != want || st != 0 {
		t.Errorf("counting over the promotion = %q (status %d), want %q at 0", out, st, want)
	}
	// Counting back over the elements the name has, of which a string has
	// none: the subscript lands before the first and is refused.
	out, st = runPromotedNegative(t, No, src)
	if !strings.Contains(out, "bad array subscript") || st == 0 {
		t.Errorf("counting over the elements = %q (status %d), want the subscript refused", out, st)
	}
	// And where no dialect answered, the refusal names the axis rather than
	// picking one of the two, and nothing is written.
	out, st = runPromotedNegative(t, Unspecified, `a=abc; a[-1]=x`)
	want := "sh: a negative subscript counting back over a scalar a write is promoting: " +
		"the shells disagree here and no dialect was chosen\n"
	if out != want || st != 2 {
		t.Errorf("unanswered = %q (status %d), want %q at 2", out, st, want)
	}
	out, st = runPromotedNegative(t, Unspecified, src)
	if !strings.HasPrefix(out, want) || !strings.HasSuffix(out, "[abc] n=1\n") {
		t.Errorf("unanswered = %q (status %d), want the refusal and the value untouched", out, st)
	}
}

// A non-negative subscript lands at the number it names under either reading,
// so it asks nothing — which is what keeps the axis off the common path.
func TestAPlainSubscriptOverAPromotedScalarAsksNothing(t *testing.T) {
	out, st := runPromotedNegative(t, Unspecified,
		`a=abc; a[1]=x; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if want := "[abc][x] n=2\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q at 0", out, st, want)
	}
	// And an unset name has nothing to promote, so the negative spelling is
	// refused in both columns without anyone being asked.
	out, st = runPromotedNegative(t, Unspecified, `unset a; a[-1]=x; echo reached`)
	if !strings.Contains(out, "bad array subscript") || st == 0 {
		t.Errorf("unset = %q (status %d), want the subscript refused", out, st)
	}
}

// runPromotedNegative runs src with one answer for the promotion axis and the
// core's answers for everything else.
func runPromotedNegative(t *testing.T, promoted Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := CoreSemantics()
		// The two neighbours these rows walk past, answered so that the axis
		// under test is the only one that can decide anything: a subscript on
		// a string is an element rather than a character, and a subscript
		// past the first element is refused rather than placed in front of
		// it. Either left unanswered refuses ahead of the question here.
		sem.ScalarSubscriptIsACharacter = No
		sem.NegativeSubscriptPastTheStartInserts = No
		sem.ArrayBaseIsZero = Yes
		sem.NegativeSubscriptCountsOverAPromotedScalar = promoted
		r.Semantics = &sem
	})
}
