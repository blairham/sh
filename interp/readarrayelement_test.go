// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// readArrayElement runs src with the `-A` spelling of the array letter and
// Semantics.ReadArrayIntoAnElementRefillsTheArray under the test's control,
// over the core vector with the neighboring `read` questions answered flat.
func readArrayElement(t *testing.T, src string, refill interp.Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
	}, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.ReadOptions = "rA"
		sem.LastPipelineElementInCurrentShell = interp.Yes
		sem.TrailingSeparatorEndsAField = interp.No
		sem.ReadTrailingWhitespaceEndsAField = interp.No
		sem.ReadNoFieldsIsOneEmptyElement = interp.No
		sem.UnsetNameAtIsOneEmptyField = interp.No
		sem.ArrayBaseIsZero = interp.Yes
		sem.ArraysAreSparse = interp.No
		sem.StoreOperandWholeArraySubscript = interp.StoreOperandWholeArraySubscriptNamesEveryElement
		sem.ReadArrayIntoAnElementRefillsTheArray = refill
		r.Semantics = &sem
	})
}

const showElementB = `printf '%s:' "${#b[@]}"; for e in "${b[@]}"; do printf '[%s]' "$e"; done; echo`

// The axis is asked only where `read -A` names one element of an indexed
// array, so the lines that name no element run under a vector that does not
// answer it — and say nothing about it (#6869).
func TestAReadArrayNamingNoElementAsksNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a plain name",
			`printf 'p q\n' | { read -A b; ` + showElementB + `; }`,
			"2:[p][q]\n",
		},
		{
			"a plain name over an array",
			`b=(x y z); printf 'p\n' | { read -A b; ` + showElementB + `; }`,
			"1:[p]\n",
		},
		{
			"a plain name over a string",
			`b=s; printf 'p q\n' | { read -A b; ` + showElementB + `; }`,
			"2:[p][q]\n",
		},
		{
			// The whole-array spelling, where it names every element.
			"the whole-array spelling",
			`b=(x y z); printf 'p q\n' | { read -A 'b[@]'; ` + showElementB + `; }`,
			"2:[p][q]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := readArrayElement(t, tc.src, interp.Unspecified)
			if out != tc.want || st != 0 {
				t.Errorf("unanswered: got %q status %d, want %q at 0 with nothing said", out, st, tc.want)
			}
		})
	}
}

// An element is where the question is put, and each answer lands the words
// where it says: the refill takes the element out and fills from the first,
// keeping the rest; the other answer splices them over the element, as the
// element literal does in the dialect that splices. The read's own status is
// printed first, so a refusal cannot hide behind the status of the line after
// it.
func TestReadArrayIntoAnElementAsksWhereTheWordsGo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, refill, splice string }{
		{
			"a middle element",
			`b=(x y z); printf 'p q\n' | { read -A 'b[1]'; printf 'st=%s ' "$?"; ` + showElementB + `; }`,
			"st=0 3:[p][q][z]\n", "st=0 4:[x][p][q][z]\n",
		},
		{
			"one word over the last element",
			`b=(x y z); printf 'p\n' | { read -A 'b[2]'; printf 'st=%s ' "$?"; ` + showElementB + `; }`,
			"st=0 2:[p][y]\n", "st=0 3:[x][y][p]\n",
		},
		{
			"a name holding nothing",
			`printf 'p q\n' | { read -A 'b[1]'; printf 'st=%s ' "$?"; ` + showElementB + `; }`,
			"st=0 2:[p][q]\n", "st=0 3:[][p][q]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := readArrayElement(t, tc.src, interp.Yes); out != tc.refill {
				t.Errorf("refilled: got %q, want %q", out, tc.refill)
			}
			if out, _ := readArrayElement(t, tc.src, interp.No); out != tc.splice {
				t.Errorf("spliced: got %q, want %q", out, tc.splice)
			}
			// And unanswered, the read is refused at 2 by the axis's own
			// question, with nothing stored either way.
			out, _ := readArrayElement(t, tc.src, interp.Unspecified)
			if !strings.Contains(out, "`read -A` into an indexed array's element") || !strings.Contains(out, "st=2 ") {
				t.Errorf("unanswered: got %q, want the axis's question and st=2", out)
			}
		})
	}
}
