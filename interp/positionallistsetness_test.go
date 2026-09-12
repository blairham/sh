// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether `$@` with no positional parameters is a *set* parameter, which is
// what a colon-less conditional asks about it.
//
// Two columns of the panel say set and four say unset, and one answer decides
// the whole family: the two conditionals, the error form, and whether the
// assignment fires at all. `$*` goes the same way, which is why the axis is
// about the list and not about one spelling (#1941).
func TestWhetherAnEmptyPositionalListIsSet(t *testing.T) {
	for _, tc := range []struct {
		name, src, isSet, isUnset string
	}{
		{"the default form", `set --; printf '[%s]' "${@-word}"`, `[]`, `[word]`},
		{"the alternate form", `set --; printf '[%s]' "${@+word}"`, `[word]`, `[]`},
		{"the star spelling of each", `set --; printf '[%s][%s]' "${*-word}" "${*+word}"`, `[][word]`, `[word][]`},
		{"unquoted", `set --; printf '[%s]' ${@-word}`, `[]`, `[word]`},
		// The colon form fires on an empty value whichever way the set-ness
		// reads, so every column answers it alike and it asks nothing.
		{"the colon forms", `set --; printf '[%s][%s]' "${@:-word}" "${@:+word}"`, `[word][]`, `[word][]`},
		// A positional parameter that is not there is unset in every column,
		// which is what says this is about the list.
		{"a positional that is not there", `set --; printf '[%s]' "${1-word}"`, `[word]`, `[word]`},
		// With anything in the list, all six call it set.
		{"with a parameter in it", `set -- p; printf '[%s][%s]' "${@-word}" "${@+word}"`, `[p][word]`, `[p][word]`},
		// The length is zero either way, so the axis is not the count.
		{"the length", `set --; printf '[%s]' "${#@}"`, `[0]`, `[0]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.isSet}, {No, tc.isUnset}} {
				sem := testSemantics()
				sem.PositionalListWithNoneIsSet = side.answer
				out, st := run(t, tc.src, withSem(sem))
				if out != side.want || st != 0 {
					t.Errorf("%v: %s = %q (status %d), want %q at 0",
						side.answer, tc.src, out, st, side.want)
				}
			}
		})
	}
}

// The assignment is the operator the answer is loudest on: where the list is
// unset the operator fires, and `$@` is a name nothing can assign to, so the
// script stops.
//
// That refusal is #1541's and is reached by every dialect the moment the
// operator fires; what this pins is that the *firing* is the axis's.
func TestTheAssignmentOnAnEmptyPositionalListFollowsTheAxis(t *testing.T) {
	const src = `set --; printf '<%s>' ${@=abc}; echo after`
	sem := testSemantics()
	sem.PositionalListWithNoneIsSet = Yes
	out, st := run(t, src, withSem(sem))
	if out != "<>after\n" || st != 0 {
		t.Errorf("set: got %q (status %d), want the operator not to fire", out, st)
	}

	sem = testSemantics()
	sem.PositionalListWithNoneIsSet = No
	out, _ = run(t, src, withSem(sem))
	if !strings.Contains(out, "@") || strings.Contains(out, "after") {
		t.Errorf("unset: got %q, want the operator to fire and the script to stop", out)
	}
}

// The axis is asked at the colon-less conditional on the list itself, and
// nowhere else.
//
// The rows that must stay quiet are the whole point: a bare `$@`, an operator
// that has no test, the colon forms, a list with something in it, and a
// subscripted name that merely happens to be spelled `@`.
func TestThePositionalListAxisIsAskedOnlyAtAColonlessTest(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"a colon-less default", `set --; printf '[%s]' "${@-word}"`, true},
		{"a colon-less alternate", `set --; printf '[%s]' "${@+word}"`, true},
		{"the star spelling", `set --; printf '[%s]' "${*-word}"`, true},
		{"the assignment", `set --; printf '[%s]' "${@=abc}"`, true},
		{"a bare expansion", `set --; printf '[%s]' "${@}"`, false},
		{"a trim", `set --; printf '[%s]' "${@%x}"`, false},
		{"the length", `set --; printf '[%s]' "${#@}"`, false},
		{"a colon form", `set --; printf '[%s]' "${@:-word}"`, false},
		{"a list with something in it", `set -- p; printf '[%s]' "${@-word}"`, false},
		{"a positional that is not there", `set --; printf '[%s]' "${1-word}"`, false},
		{"an ordinary name", `unset v; printf '[%s]' "${v-word}"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.PositionalListWithNoneIsSet = Unspecified
			out, _ := run(t, tc.src, withSem(sem))
			said := strings.Contains(out, "no positional parameters")
			if said != tc.refused {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "reported", false: "not reported"}[tc.refused])
			}
		})
	}
}
