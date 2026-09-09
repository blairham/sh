// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestAProducedArrayReadsLikeAStoredOneWithoutASubscript is #1600, and it is
// written as a comparison rather than as a table of expected strings.
//
// A produced array was reachable only through a subscript. `${p[@]}` and
// `${#p[@]}` were right, and every reading of the bare name behaved as though
// nothing had ever been registered under it: the value empty, the length 0,
// a `:-` default taken, and `${p+SET}` reporting the parameter absent.
//
// What a bare array name *means* is an axis — the whole list joined in one
// shell, the first element in the others — so the assertion here is not what
// the answer should be. It is that the produced array and a stored array
// holding the same elements come to the *same* answer, under either setting
// of the axis. That is the whole of the defect: nothing about producing a
// value is supposed to change what reading it without a subscript means.
func TestAProducedArrayReadsLikeAStoredOneWithoutASubscript(t *testing.T) {
	elems := []string{"one", "two", "three"}
	for _, whole := range []Answer{Yes, No} {
		for _, tc := range []struct {
			name, expr string
		}{
			{"the value", `$p`},
			{"the value, braced", `${p}`},
			{"the length", `${#p}`},
			{"a default is not taken", `${p:-DEFAULT}`},
			{"the alternate says it is set", `${p+SET}`},
			{"the subscripted value, as a control", `${p[@]}`},
			{"the subscripted length, as a control", `${#p[@]}`},
		} {
			t.Run(tc.name+"/"+whole.String(), func(t *testing.T) {
				src := `echo "` + tc.expr + `"`
				produced, _ := runGrammar(t, src, nil, func(r *Runner) {
					setWholeArrayAxis(r, whole)
					r.SetDynamicArray("p", func(*Runner) []string { return elems })
				})
				stored, _ := runGrammar(t, `p=(one two three); `+src, nil, func(r *Runner) {
					setWholeArrayAxis(r, whole)
				})
				if produced != stored {
					t.Errorf("produced %q, stored %q — a produced array must read as a stored one does",
						produced, stored)
				}
				// And neither is allowed to be the empty answer the bug gave,
				// which is what keeps this from passing on two broken halves.
				if strings.TrimSpace(stored) == "" {
					t.Fatalf("the control itself is empty for %s — the comparison proves nothing", tc.expr)
				}
			})
		}
	}
}

// TestAProducedArrayIsSetForTheParameterTests is the half a comparison
// against a stored array cannot reach, because `-v` and `${+…}` are asked of
// the *name* and a stored array with elements is trivially there.
//
// A registered parameter reporting that it does not exist is worse in kind
// than a wrong value: a script that guards on the test takes the wrong branch
// on purpose. Measured in bash 5.3.15, `[[ -v FUNCNAME ]]` inside a function
// is true, and it was false here.
//
// The empty producer is the row that has two answers, and they are the axis
// rather than a bug in either shell. A shell that reads a bare array name as
// the whole list has a value for an empty one — the empty join — so the name
// is set; a shell that reads the base element has no element zero, so it is
// not. Measured with nothing to produce: zsh answers `${+funcstack}` 1 at a
// top level where `$#funcstack` is 0, and bash answers `${FUNCNAME+SET}`
// empty outside a call.
func TestAProducedArrayIsSetForTheParameterTests(t *testing.T) {
	for _, tc := range []struct {
		name, src       string
		whole, notWhole string
	}{
		{"a producer with elements", `echo "${p+SET}[end]"`, "SET[end]", "SET[end]"},
		{"an empty producer", `echo "${e+SET}[end]"`, "SET[end]", "[end]"},
		{"a name with no producer", `echo "${nope+SET}[end]"`, "[end]", "[end]"},
	} {
		for _, whole := range []Answer{Yes, No} {
			want := tc.whole
			if whole == No {
				want = tc.notWhole
			}
			t.Run(tc.name+"/"+whole.String(), func(t *testing.T) {
				out, _ := runGrammar(t, tc.src, nil, func(r *Runner) {
					setWholeArrayAxis(r, whole)
					r.SetDynamicArray("p", func(*Runner) []string { return []string{"x"} })
					r.SetDynamicArray("e", func(*Runner) []string { return nil })
				})
				if got := strings.TrimSpace(out); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

// TestAnEmptyAssociationAnswersSetOnTheSameAxis is the table half of the row
// above, and it reaches a *stored* one — which was wrong in this shell too,
// and had nothing to do with producing a value.
//
// `assocScalar` returned "not set" for an empty table before it asked the
// axis at all, so no dialect could answer otherwise: measured, zsh 5.9.2
// answers `typeset -A h; ${+h}` with 1 and this shell answered 0, while
// bash's `declare -A h; [[ -v h ]]` is false and was right by accident.
func TestAnEmptyAssociationAnswersSetOnTheSameAxis(t *testing.T) {
	for _, tc := range []struct {
		whole Answer
		want  string
	}{
		{Yes, "SET[end]"},
		{No, "[end]"},
	} {
		t.Run(tc.whole.String(), func(t *testing.T) {
			// The stored table is made on the runner rather than with
			// `declare`, which is a dialect's builtin and not the core's —
			// this test names an axis, so it may not name a shell.
			for _, src := range []string{
				`echo "${h+SET}[end]"`,
				`echo "${d+SET}[end]"`,
			} {
				out, _ := runGrammar(t, src, nil, func(r *Runner) {
					setWholeArrayAxis(r, tc.whole)
					r.AssocArrays = map[string]AssocArray{"h": {}}
					r.SetDynamicAssoc("d", func(*Runner) AssocArray { return nil })
				})
				if got := strings.TrimSpace(out); got != tc.want {
					t.Errorf("%s: got %q, want %q", src, got, tc.want)
				}
			}
		})
	}
}

// setWholeArrayAxis answers what a bare array name gives, on a copy so the
// preset the runner was built with is left alone.
func setWholeArrayAxis(r *Runner, whole Answer) {
	sem := *r.Semantics
	sem.ArrayScalarIsTheWholeArray = whole
	sem.ArrayNameWithoutSubscriptIsTheList = whole
	// The length is its own axis and has to move with the others, or `${#p}`
	// reads as a string width on both sides of the comparison and cannot see
	// whether the element count knows about producers at all.
	sem.ArrayLengthWithoutSubscriptIsCount = whole
	r.Semantics = &sem
}
