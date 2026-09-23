// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a quoted list expansion that produced no fields takes with it is an
// axis with three values, and all three are reachable.
//
// The probe is an empty expansion written beside `"$@"` with no positional
// parameters, on each side of it in turn: one reading takes the word whichever
// side the empty expansion is on, one takes only what stands in front of the
// list, and one takes nothing at all.
func TestEmptyListTakesTheWordIsAThreeValuedAxis(t *testing.T) {
	const set = `set --; e=; n(){ echo "$#"; }; `
	for _, tc := range []struct {
		name  string
		reach EmptyListReach
		lead  string
		trail string
	}{
		{"nothing at all", EmptyListReachNothing, "1\n", "1\n"},
		{"the whole word", EmptyListReachTheWord, "0\n", "0\n"},
		{"what stands before it", EmptyListReachWhatStandsBeforeIt, "0\n", "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask := func(s *Semantics) { s.EmptyListTakesTheWord = tc.reach }
			if out, st := axisRun(t, set+`n "$e$@"`, ask); st != 0 || out != tc.lead {
				t.Errorf(`"$e$@": got %q status %d, want %q`, out, st, tc.lead)
			}
			if out, st := axisRun(t, set+`n "$@$e"`, ask); st != 0 || out != tc.trail {
				t.Errorf(`"$@$e": got %q status %d, want %q`, out, st, tc.trail)
			}
		})
	}
	if _, st := axisRun(t, set+`n "$e$@"`, func(*Semantics) {}); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

// And it is asked only where the three readings part, which is what keeps it
// away from every word a script writes.
//
// Three shapes reach the list and none of them is a question: a word with text
// in it is a field in every column, a word nothing else reached is no field in
// every column, and `"$*"` is not a list at all.
func TestTheEmptyListAxisIsNotAskedWhereTheReadingsAgree(t *testing.T) {
	const set = `set --; unset xxx; e=; n(){ echo "$#"; }; `
	for _, tc := range []struct{ name, src, want string }{
		{"the list on its own", set + `n "$@"`, "0\n"},
		{"a literal in the word", set + `n "x$@"`, "1\n"},
		{"a literal behind it", set + `n "$@x"`, "1\n"},
		{"a value that is not empty", set + `q=QQ; n "$q$@"`, "1\n"},
		{"the star is not a list", set + `n "$xxx${*}"`, "1\n"},
		{"an unquoted empty neighbor", set + `n $e$@`, "0\n"},
		{"the list with parameters in it", `set -- a b; e=; n(){ echo "$#"; }; n "$e$@"`, "2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := axisRun(t, tc.src, func(*Semantics) {}); st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q with no question asked", tc.src, out, st, tc.want)
			}
		})
	}
}

// The reading is about one quoted **string** rather than about the word, so an
// expansion in a pair of quotes of its own brings the word back under every
// reading — which is the half that cannot be seen from the spans alone, since
// `"$e$@"` and `"$e""$@"` are the same two spans.
//
// Asserted under the reading that takes the most, because that is the one these
// rows have to survive: under it `"$e$@"` is no field and `"$e""$@"` is one.
func TestAQuotedRunOfItsOwnBringsTheWordBack(t *testing.T) {
	const set = `set --; e=; n(){ echo "$#"; }; `
	ask := func(s *Semantics) { s.EmptyListTakesTheWord = EmptyListReachTheWord }
	for _, tc := range []struct{ name, src, want string }{
		{"in its own quotes in front", `n "$e""$@"`, "1\n"},
		{"and behind", `n "$@""$e"`, "1\n"},
		{"a quoted null literal", `n "$@"''`, "1\n"},
		{"and an empty pair of double quotes", `n "$@"""`, "1\n"},
		// The contrast, which is the same two expansions inside one pair of
		// quotes. Without it the rows above would pass for a reading that had
		// simply stopped firing.
		{"the same expansions in one pair", `n "$e$@"`, "0\n"},
		{"and the other way round", `n "$@$e"`, "0\n"},
		// A braced spelling is measured the same way, and its span is four
		// characters wide rather than two — which is what says the boundary is
		// read from the source and not from a span count.
		{"a braced list in one pair", `n "$e${@}"`, "0\n"},
		{"and a braced list in its own", `n "$e""${@}"`, "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := axisRun(t, set+tc.src, ask); st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// An empty array's `[@]` is the same question as `$@`, which is why the axis is
// named for a list rather than for the positional parameters.
func TestAnEmptyArrayAnswersTheSameAxis(t *testing.T) {
	const set = `a=(); e=; n(){ echo "$#"; }; `
	for _, tc := range []struct {
		name  string
		reach EmptyListReach
		lead  string
		trail string
	}{
		{"nothing at all", EmptyListReachNothing, "1\n", "1\n"},
		{"the whole word", EmptyListReachTheWord, "0\n", "0\n"},
		{"what stands before it", EmptyListReachWhatStandsBeforeIt, "0\n", "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ask := func(s *Semantics) {
				s.EmptyListTakesTheWord = tc.reach
				s.ArrayBaseIsZero = Yes
			}
			if out, st := axisRun(t, set+`n "$e${a[@]}"`, ask); st != 0 || out != tc.lead {
				t.Errorf(`"$e${a[@]}": got %q status %d, want %q`, out, st, tc.lead)
			}
			if out, st := axisRun(t, set+`n "${a[@]}$e"`, ask); st != 0 || out != tc.trail {
				t.Errorf(`"${a[@]}$e": got %q status %d, want %q`, out, st, tc.trail)
			}
		})
	}
}
