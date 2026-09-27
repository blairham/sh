// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The counter every row here goes through.
//
// It prints the **count** and then the fields in brackets, because `echo` is
// the instrument this question cannot be measured with: it joins its
// arguments with a blank, so `[a] []` and `[a]` print as `a ` and `a` and a
// regression row written with it could not fail. See #4800.
const emptyAltCounter = `f(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; printf '\n'; }` + "\n"

// braceEmptyAlt runs one snippet under a vector answering
// BraceEmptyAlternativeIsAField the given way, with every other brace axis the
// rows reach pinned so that nothing here is decided by a refusal.
func braceEmptyAlt(t *testing.T, src string, keep Answer) (string, int) {
	t.Helper()
	out, st := run(t, emptyAltCounter+src+"\n", func(r *Runner) {
		s := *r.Semantics
		s.BraceExpansion = Yes
		s.BraceEmptyAlternativeIsAField = keep
		s.BraceOutputRereadAsText = No
		s.BraceRescanEntersFailedGroup = No
		s.BraceBodyReadAfterExpansion = No
		s.BraceFanExpandsEachNameOnItsOwn = No
		r.Semantics = &s
	})
	return strings.TrimSpace(out), st
}

// An alternative that came to nothing is a field of its own in two of the four
// columns that have braces, and is removed with the word it left empty in the
// other two. See interp/braceemptyalt.go for the panel.
func TestABraceAlternativeThatCameToNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, kept, dropped string }{
		{"a written empty alternative", `f {a,}`, `2 | [a] []`, `1 | [a]`},
		{"in front", `f {,a}`, `2 | [] [a]`, `1 | [a]`},
		{"both of them", `f {,}`, `2 | [] []`, `0 |`},
		{"in the middle of three", `f {a,,b}`, `3 | [a] [] [b]`, `2 | [a] [b]`},
		{"one an unset parameter emptied", `unset u; f {a,$u}`, `2 | [a] []`, `1 | [a]`},
		{"one an empty parameter emptied", `e=; f {a,$e}`, `2 | [a] []`, `1 | [a]`},
		{"one a substitution emptied", `f {a,$(true)}`, `2 | [a] []`, `1 | [a]`},
		{"a nested group's", `f {a,{b,}}`, `3 | [a] [b] []`, `2 | [a] [b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := braceEmptyAlt(t, tc.src, Yes); out != tc.kept || st != 0 {
				t.Errorf("kept: %s = %q status %d, want %q", tc.src, out, st, tc.kept)
			}
			if out, st := braceEmptyAlt(t, tc.src, No); out != tc.dropped || st != 0 {
				t.Errorf("dropped: %s = %q status %d, want %q", tc.src, out, st, tc.dropped)
			}
		})
	}
}

// The rows the axis must not reach: the question is about an **alternative**
// that came to nothing, and every one of these is a word that came to
// something, or a word that went away for a reason of its own.
//
// Each is run under both readings and must answer the same either way, which
// is what says the rule is keyed on the alternative rather than on "a name
// that produced no field" — a rule keyed the second way answers the
// distributive row `2 | [p] [q]` under one reading and `0 |` under the other,
// and zsh 5.9.2 measures `0 |`.
func TestABraceNameThatCameToNothingForAReasonOfItsOwn(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a name with text in it", `unset u; f {a,b}$u`, `2 | [a] [b]`},
		{"a quoted empty alternative is a field in every column", `f {a,""}`, `2 | [a] []`},
		{"and an empty one beside text", `f x{a,}y`, `2 | [xay] [xy]`},
		{"a list that produced nothing", `set --; f {p,q}$@`, `2 | [p] [q]`},
		{"a group that is not a list at all", `f {a}`, `1 | [{a}]`},
		{"a quoted group", `f "{a,}"`, `1 | [{a,}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept, st := braceEmptyAlt(t, tc.src, Yes)
			if kept != tc.want || st != 0 {
				t.Errorf("kept: %s = %q status %d, want %q", tc.src, kept, st, tc.want)
			}
			dropped, st := braceEmptyAlt(t, tc.src, No)
			if dropped != tc.want || st != 0 {
				t.Errorf("dropped: %s = %q status %d, want %q", tc.src, dropped, st, tc.want)
			}
		})
	}
}

// A vector that has not answered is refused where an alternative came to
// nothing, and is asked nothing at all where none did.
//
// The second half is the one worth a test: a question whose two answers are
// the same answer must not be the thing that refuses a script, so every brace
// group whose alternatives are text runs under a vector that never answered
// this.
func TestAnUnansweredEmptyBraceAlternative(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"an empty alternative is the disagreement", `f {a,}`, true},
		{"and one an expansion emptied is the same one", `unset u; f {a,$u}`, true},
		{"alternatives that are text are not", `f {a,b}`, false},
		{"nor is a group that is not a list", `f {a}`, false},
		{"nor a word with no brace in it", `f a b`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := braceEmptyAlt(t, tc.src, Unspecified)
			if tc.refused {
				if st == 0 {
					t.Fatalf("%s = %q status 0, want a refusal", tc.src, out)
				}
				if !strings.Contains(out, "brace alternative") {
					t.Errorf("%s refused with %q, want the axis named", tc.src, out)
				}
				return
			}
			if st != 0 {
				t.Errorf("%s = %q status %d, want no question put", tc.src, out, st)
			}
		})
	}
}
