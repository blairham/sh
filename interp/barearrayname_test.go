// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// runBareName runs src with the axis that decides what a plain `$a` reads, and
// with the two axes a removed element needs to be reachable at all: the base,
// which says which numeral names the first element, and the span policy, which
// says whether `unset` takes an element out or blanks it.
func runBareName(t *testing.T, whole Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayScalarIsTheWholeArray = whole
		sem.ArrayLengthWithoutSubscriptIsCount = No
		if whole == Yes {
			sem.ArrayBaseIsZero = No
			sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
			sem.ArraysAreSparse = No
		} else {
			sem.ArrayBaseIsZero = Yes
			sem.UnsetArraySpan = UnsetArraySpanRemovesTheElements
			sem.ArraysAreSparse = Yes
		}
		r.Semantics = &sem
	})
}

// A bare array name is the element at the *base* position under the reading
// that takes one element, so removing that element leaves the name unset —
// even though the array still holds the rest. It read the lowest subscript
// still assigned, so `a=(x y z); unset "a[0]"` answered `y`: an element the
// script never named, at a position it had not asked about.
func TestABareArrayNameIsUnsetOnceItsBaseElementIsRemoved(t *testing.T) {
	out, st := runBareName(t, No,
		`a=(x y z); unset "a[0]"; echo "bare=[${a-U}] set=[${a+S}] at=[${a[@]}] n=${#a[@]}"`)
	if want := "bare=[U] set=[] at=[y z] n=2\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The other reading answers with the list, so the same line leaves the name
// set — and this is the half a fix aimed at one shell breaks. Under that
// reading the base is 1, so the element removed is the second one and what is
// left in its place is an empty string rather than a gap.
func TestABareArrayNameUnderTheWholeArrayReadingStaysSet(t *testing.T) {
	out, st := runBareName(t, Yes,
		`a=(x y z); unset "a[2]"; echo "bare=[${a-U}] set=[${a+S}] n=${#a[@]}"`)
	if want := "bare=[x  z] set=[S] n=3\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// Nothing was removed here: the base element was never written. It is the
// position and not the removal that decides, which the lowest-assigned reading
// got wrong the same way — `a[5]=q` answered `q`.
func TestABareArrayNameIsUnsetWhenTheBaseElementWasNeverWritten(t *testing.T) {
	out, st := runBareName(t, No, `a[5]=q; echo "bare=[${a-U}] whole=[${a[@]+S}] n=${#a[@]}"`)
	if want := "bare=[U] whole=[S] n=1\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// An arithmetic reference reads the same view, where an unset name is 0 rather
// than a diagnostic — so the wrong reading is a wrong number and nothing says
// so. `$((a+5))` was 7, five plus the element that had survived.
func TestABareArrayNameInArithmeticIsZeroOnceItsBaseElementIsRemoved(t *testing.T) {
	out, st := runBareName(t, No, `a=(1 2 3); unset "a[0]"; echo "arith=$((a+5))"`)
	if want := "arith=5\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The loud form: with the name unset, `set -u` refuses to expand it and the
// command fails. It printed a value at status 0, so the option a script turns
// on precisely to catch this said nothing.
func TestABareArrayNameWithNoBaseElementIsRefusedUnderNounset(t *testing.T) {
	out, st := runBareName(t, No, `a=(x y z); unset "a[0]"; set -u; echo "[$a]"; echo after`)
	// The whole of the output, so that neither the value nor the command
	// after it can be there: the wording and status are the core's, and the
	// point is that the expansion refused rather than answering `[y]`.
	if want := "sh: a: parameter not set\n"; out != want {
		t.Errorf("out = %q, want exactly %q", out, want)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2 — the refusal ends the script", st)
	}
}

// An array with no elements has no base element either, so the two readings
// part over it as well: one has nothing to read and one reads an empty list.
// Both halves in one test, because a fix for either alone is what produced the
// set-but-empty answer neither shell gives.
func TestABareArrayNameOfAnArrayWithNothingInIt(t *testing.T) {
	const src = `a=(x); unset "a[1]"; echo "bare=[${a-U}] set=[${a+S}] n=${#a[@]}"`
	out, st := runBareName(t, Yes, src)
	if want := "bare=[] set=[S] n=1\n"; out != want {
		t.Errorf("whole-array reading: out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("whole-array reading: status = %d, want 0", st)
	}
	out, st = runBareName(t, No, `a=(x); unset "a[0]"; echo "bare=[${a-U}] set=[${a+S}] n=${#a[@]}"`)
	if want := "bare=[U] set=[] n=0\n"; out != want {
		t.Errorf("one-element reading: out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("one-element reading: status = %d, want 0", st)
	}
}

// Writing the base element back makes the name set again, which is what says
// the store and not a flag is what is being read.
func TestABareArrayNameComesBackWhenItsBaseElementIsWrittenAgain(t *testing.T) {
	out, st := runBareName(t, No, `a=(x y z); unset "a[0]"; a[0]=q; echo "bare=[${a-U}]"`)
	if want := "bare=[q]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
