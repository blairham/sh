// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The two axes `unset` gained for #2373: whether a name the shell has never
// heard of has its brackets read at all, and whether a later operand's
// subscript overwrites the status an earlier one failed with.
//
// Both are disagreements between shells that reach this code rather than one
// shell's fact, which is what the issue got wrong on both counts and what the
// re-measurement of 2026-09-12 corrected. Every row below is measured on the
// live panel, `env -i PATH=/usr/bin:/bin`, from a script file.

// unsetAxisRun answers everything the operand loop needs *except* the two
// axes under test, so a row can vary one of them alone.
func unsetAxisRun(t *testing.T, src string, skip, last Answer) (string, int) {
	t.Helper()
	out, errs, st := declRun(t, src, func(sem *Semantics) {
		sem.UnsetTakesASubscript = Yes
		sem.ArrayBaseIsZero = No
		sem.ArraysAreSparse = Yes
		sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
		sem.BadSubscriptToUnsetFatal = No
		sem.UnsetSubscriptSkippedWhenNameUnset = skip
		sem.UnsetStatusIsTheLastSubscripts = last
	}, Diagnostics{})
	return out + errs, st
}

// TestUnsetSkipsTheSubscriptOfANameItHasNotGot is the first axis, and the
// rows are chosen so that the two readings cannot agree by accident.
//
// The arithmetic *error* is the weaker half of the evidence: a shell that
// evaluated the subscript and swallowed the complaint would look the same. The
// side effect is what separates them — `i=0; unset "nodecl[i++]"` leaves i at
// 0 under Yes and at 1 under No, with status 0 either way — so the row that
// carries the axis is the one with no error in it at all.
func TestUnsetSkipsTheSubscriptOfANameItHasNotGot(t *testing.T) {
	for _, tc := range []struct {
		name, src  string
		yes, no    string
		yesSt, noS int
	}{
		{
			"a name that was never assigned",
			`unset "nope[x+]"; printf "st=%d" "$?"`,
			"st=0", "st=1", 0, 0,
		},
		{
			"a name that was unset a moment ago",
			`a=(x y z); unset a "a[x+]"; printf "st=%d" "$?"`,
			"st=0", "st=1", 0, 0,
		},
		{
			// The row with no error in it: only the side effect tells the
			// readings apart, so a fix that merely silenced the complaint
			// would pass every other row and fail this one.
			"and the subscript is not evaluated, not merely forgiven",
			`i=0; unset "nodecl[i++]"; printf "i=%s st=%d" "$i" "$?"`,
			"i=0 st=0", "i=1 st=0", 0, 0,
		},
		{
			// Set-ness and not emptiness. Three shapes that all count as
			// heard of, measured a name at a time on zsh.
			"an empty scalar is a name it has heard of",
			`v=; unset "v[x+]"; printf "st=%d" "$?"`,
			"st=1", "st=1", 0, 0,
		},
		{
			"and so is a declared array with nothing in it",
			`typeset -a e; unset "e[x+]"; printf "st=%d" "$?"`,
			"st=1", "st=1", 0, 0,
		},
		{
			"a plain subscript is the same either way",
			`unset "nope[1]"; printf "st=%d" "$?"`,
			"st=0", "st=0", 0, 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.yes}, {No, tc.no}} {
				out, st := unsetAxisRun(t, tc.src, a.answer, No)
				got := strings.TrimSpace(out)
				if !strings.Contains(got, a.want) {
					t.Errorf("%s under %v = %q (status %d), want %q",
						tc.src, a.answer, got, st, a.want)
				}
			}
		})
	}
}

// TestUnsetStatusIsTheLastSubscripts is the second axis.
//
// It is the last *subscript's* and not the last operand's, which is the
// correction the issue's two-row table would have missed: a plain name, a
// name the shell has not got and an association's key all leave the status
// where they found it, so an earlier failure still stands behind them.
func TestUnsetStatusIsTheLastSubscripts(t *testing.T) {
	for _, tc := range []struct{ name, src, yes, no string }{
		{
			"a later element clears the failure",
			`a=(x y z); unset "a[x+]" "a[1]"; printf "st=%d" "$?"`,
			"st=0", "st=1",
		},
		{
			// The other order, which no reading disagrees about — and
			// without it the row above cannot tell "the last one wins"
			// from "any success wins".
			"and the other order fails under both",
			`a=(x y z); unset "a[1]" "a[x+]"; printf "st=%d" "$?"`,
			"st=1", "st=1",
		},
		{
			"the whole array carries a status too",
			`a=(x y z); unset "a[x+]" "a[@]"; printf "st=%d" "$?"`,
			"st=0", "st=1",
		},
		{
			"and an element that is not there",
			`a=(x y z); unset "a[x+]" "a[9]"; printf "st=%d" "$?"`,
			"st=0", "st=1",
		},
		{
			"a plain name does not",
			`a=(x y z); unset "a[x+]" nope; printf "st=%d" "$?"`,
			"st=1", "st=1",
		},
		{
			"nor does the array's own name",
			`a=(x y z); unset "a[x+]" a; printf "st=%d" "$?"`,
			"st=1", "st=1",
		},
		{
			"nor an association's key, which is not arithmetic",
			`typeset -A m; m[k]=1; a=(x); unset "a[x+]" "m[k]"; printf "st=%d" "$?"`,
			"st=1", "st=1",
		},
		{
			"and two failures are still a failure",
			`a=(x y z); unset "a[x+]" "a[y+]"; printf "st=%d" "$?"`,
			"st=1", "st=1",
		},
		{
			"as is a success between two of them",
			`a=(x y z); unset "a[x+]" "a[1]" "a[y+]"; printf "st=%d" "$?"`,
			"st=1", "st=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.yes}, {No, tc.no}} {
				out, st := unsetAxisRun(t, tc.src, No, a.answer)
				got := strings.TrimSpace(out)
				if !strings.Contains(got, a.want) {
					t.Errorf("%s under %v = %q (status %d), want %q",
						tc.src, a.answer, got, st, a.want)
				}
			}
		})
	}
}

// TestTheTwoUnsetAxesComposeAsTheyDoInZsh is the one row that needs both:
// a name the shell has not got is skipped, and being skipped it carries no
// status, so an earlier subscript's failure survives it.
//
// Measured on zsh 5.9.2 — `b=(x y z); unset "b[x+]" "nope[1]"` is 1, where
// `unset "b[x+]" "b[1]"` is 0.
func TestTheTwoUnsetAxesComposeAsTheyDoInZsh(t *testing.T) {
	src := `b=(x y z); unset "b[x+]" "nope[1]"; printf "st=%d" "$?"`
	out, _ := unsetAxisRun(t, src, Yes, Yes)
	if got := strings.TrimSpace(out); !strings.Contains(got, "st=1") {
		t.Errorf("%s = %q, want st=1 — a skipped operand carries no status", src, got)
	}
}
