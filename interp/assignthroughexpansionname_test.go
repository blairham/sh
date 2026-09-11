// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// assigningThroughAnExpansion answers the two axes this family reaches, so a
// row below fails over the name it is about rather than over an unanswered
// axis underneath it. The positional answer is the one every row states for
// itself.
func assigningThroughAnExpansion(positional Answer, dg Diagnostics) func(*Runner) {
	return func(r *Runner) {
		sem := *r.Semantics
		sem.AssignThroughExpansionMayNameAPositional = positional
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics = &sem
		if dg.AssignThroughExpansionBadName != "" {
			d := dg
			r.Diagnostics = &d
		}
	}
}

// TestAnAssignmentThroughAnExpansionRefusesAParameterItCannotName is #1541.
//
// `${name::=word}` already went through the name check and the conditional
// `${name:=word}` beside it did not: it called setVar on whatever name it was
// given, so `set --; printf "<%s>" ${@:=abc}` substituted `abc` at status 0
// where every shell in the panel refuses fatally. That is the failure shape
// this codebase minds most — a plausible value where the shell stopped — and
// a later read of the parameter finds nothing behind the substituted word.
//
// `@` and `*` are unanimous, so they are asserted with the axis set either
// way: what refuses them is not the positional question.
func TestAnAssignmentThroughAnExpansionRefusesAParameterItCannotName(t *testing.T) {
	for _, positional := range []Answer{Yes, No} {
		for _, tc := range []struct{ name, src, want string }{
			{"the whole list", `set --; printf "<%s>" ${@:=abc}`, "not an identifier: @"},
			{"and its joining spelling", `set --; printf "<%s>" ${*:=abc}`, "not an identifier: *"},
			// A word with text around the expansion, so the refusal is not
			// something only a word that is nothing else can reach.
			{"inside a larger word", `set --; printf "<%s>" x${@:=abc}y`, "not an identifier: @"},
		} {
			t.Run(tc.name+"/"+positional.String(), func(t *testing.T) {
				out, st := runGrammar(t, tc.src+`; echo AFTER`, alwaysAssigning,
					assigningThroughAnExpansion(positional, Diagnostics{}))
				if !strings.Contains(out, tc.want) {
					t.Errorf("output = %q, want %q in it", out, tc.want)
				}
				if strings.Contains(out, "AFTER") || st == 0 {
					t.Errorf("output = %q (status %d), want the refusal to stop the line", out, st)
				}
			})
		}
	}
}

// TestAPositionalAssignedThroughAnExpansionIsAnAxis: five columns refuse
// `${1:=abc}` and one assigns, which is more than a wording swap — so it is
// asked rather than decided.
//
// The assertion on the Yes side is on `$1` afterwards rather than on what the
// expansion substituted: substituting `abc` and storing nothing looks
// identical on the line itself, and that is exactly the wrong answer this
// issue is about.
func TestAPositionalAssignedThroughAnExpansionIsAnAxis(t *testing.T) {
	out, st := runGrammar(t, `set --; printf "<%s>" ${1:=abc}; printf "|one=%s" "$1"`,
		alwaysAssigning, assigningThroughAnExpansion(Yes, Diagnostics{}))
	if out != "<abc>|one=abc" || st != 0 {
		t.Errorf("assigning = %q (status %d), want the parameter stored at 0", out, st)
	}
	out, st = runGrammar(t, `set --; printf "<%s>" ${1:=abc}; echo AFTER`,
		alwaysAssigning, assigningThroughAnExpansion(No, Diagnostics{}))
	if !strings.Contains(out, "not an identifier: 1") || strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("refusing = %q (status %d), want the positional named and the line stopped", out, st)
	}
	// The *unconditional* operator does not ask the axis: the one shell that
	// can write `${1::=new}` assigns through it, so there is no second answer
	// to hold. A fix that routed both operators through the axis would refuse
	// this row wherever the axis says No.
	out, st = runGrammar(t, `set --; printf "<%s>" ${1::=new}; printf "|one=%s" "$1"`,
		alwaysAssigning, assigningThroughAnExpansion(No, Diagnostics{}))
	if out != "<new>|one=new" || st != 0 {
		t.Errorf("the unconditional operator = %q (status %d), want it to assign at 0", out, st)
	}
}

// TestTheAssignmentThroughAnExpansionCheckFiresOnlyWithTheOperator: it is a
// run-time check and not a reading of the text. `set -- p` leaves the
// parameter set, so the operator does not fire and nothing is refused —
// unanimous across the panel, and the row that keeps the check from being
// turned into a grammar rule.
func TestTheAssignmentThroughAnExpansionCheckFiresOnlyWithTheOperator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the parameter is there", `set -- p; printf "<%s>" ${@:=abc}; echo AFTER`, "<p>AFTER\n"},
		{"the expansion is never reached", `set --; if false; then echo ${@:=abc}; fi; echo AFTER`, "AFTER\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, alwaysAssigning,
				assigningThroughAnExpansion(No, Diagnostics{}))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// TestTheAssignmentThroughAnExpansionWordingTakesTwoVerbs: the name without
// its sigil, and the whole word the expansion stands in.
//
// The second verb has one reader in the panel and it needs the whole word
// rather than the braces — `x${@:=abc}y` and `"${@:=abc}"` are each blamed
// entire there — so a wording that reached for the expansion alone would name
// text the script does not contain.
func TestTheAssignmentThroughAnExpansionWordingTakesTwoVerbs(t *testing.T) {
	dg := Diagnostics{AssignThroughExpansionBadName: "%[2]s: bad substitution"}
	for _, tc := range []struct{ src, want string }{
		{`set --; printf "<%s>" ${@:=abc}`, "${@:=abc}: bad substitution"},
		{`set --; printf "<%s>" x${@:=abc}y`, "x${@:=abc}y: bad substitution"},
		{`set --; printf "<%s>" "${@:=abc}"`, `"${@:=abc}": bad substitution`},
	} {
		out, _ := runGrammar(t, tc.src, alwaysAssigning, assigningThroughAnExpansion(No, dg))
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
		}
	}
	// And the first verb on its own, which is the spelling three of the four
	// wordings use. The sigil is the dialect's to write back or leave off.
	dg = Diagnostics{AssignThroughExpansionBadName: "$%[1]s: cannot assign in this way"}
	out, _ := runGrammar(t, `set --; printf "<%s>" x${@:=abc}y`, alwaysAssigning,
		assigningThroughAnExpansion(No, dg))
	if want := "$@: cannot assign in this way"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q in it", out, want)
	}
}

// TestAnAssignmentThroughAnExpansionGivesUpWhatTheDialectGivesUp: the refusal
// is a word that could not be read rather than an expansion that failed, and
// the two are told apart by the shell that abandons only the *line*.
//
// Measured on the shell with that answer: `printf "<%s>" ${@:=abc}` on its own
// line still lets the next line run, and the same two commands separated by
// `;` do not. Routing this through the fatal-expansion door instead ended the
// shell in every dialect — and took that door's `-c` status with it, which is
// 127 in the one shell that has one and 1 in the measurement.
func TestAnAssignmentThroughAnExpansionGivesUpWhatTheDialectGivesUp(t *testing.T) {
	src := "set --\nprintf \"<%s>\" ${@:=abc}\necho AFTER"
	out, st := runGrammar(t, src, alwaysAssigning, func(r *Runner) {
		assigningThroughAnExpansion(No, Diagnostics{})(r)
		sem := *r.Semantics
		sem.FailedExpansionAbandonsTheLine = Yes
		r.Semantics = &sem
	})
	if !strings.Contains(out, "AFTER") || st != 0 {
		t.Errorf("abandoning the line = %q (status %d), want the next line to run at 0", out, st)
	}
	out, st = runGrammar(t, src, alwaysAssigning, assigningThroughAnExpansion(No, Diagnostics{}))
	if strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("ending the shell = %q (status %d), want nothing after it", out, st)
	}
}

// TestAnUnansweredPositionalAxisIsOneComplaint: a run with no dialect is told
// which axis it needs and not also told the fallback wording, which would be
// two sentences about one line — and the fallback is one shell's, which is
// the thing a core run has not got.
func TestAnUnansweredPositionalAxisIsOneComplaint(t *testing.T) {
	out, _ := runGrammar(t, `set --; printf "<%s>" ${1:=abc}`, alwaysAssigning, func(r *Runner) {
		sem := *r.Semantics
		sem.AssignThroughExpansionMayNameAPositional = Unspecified
		r.Semantics = &sem
	})
	if !strings.Contains(out, "assigning to a positional parameter") {
		t.Errorf("got %q, want the axis named", out)
	}
	if strings.Contains(out, "not an identifier") {
		t.Errorf("got %q, want only the axis complaint", out)
	}
	if strings.Contains(out, "<") {
		t.Errorf("got %q, want the construct not to run", out)
	}
}
