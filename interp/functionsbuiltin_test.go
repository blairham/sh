// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// functionsSem is the permissive base with the two letter sets answered and
// the declaration's own set wide enough for `typeset -f` to be compared
// against. These cases turn on nothing else.
//
// What they can*not* see is which dialect registers the words, or which
// letters a real one gives them — that is dialect/zsh/functions_test.go, and
// a case here passing proves nothing about it (#1386).
func functionsSem() Semantics {
	s := permissive()
	s.DeclareOptions = "aAfFgilprx"
	s.FunctionsOptions = "m"
	s.UnfunctionOptions = "m"
	s.UnsetOptions = "vfm"
	s.BadOptionToSpecialBuiltinFatal = No
	return s
}

func withFunctionNames(s Semantics) func(*Runner) {
	return func(r *Runner) {
		withSem(s)(r)
		r.Register("functions", FunctionsBuiltin())
		r.Register("unfunction", UnfunctionBuiltin())
	}
}

// TestTheSecondNamesAreTheSameCode. The point of registering rather than
// reimplementing: `functions` must answer exactly what `typeset -f` answers
// and `unfunction` must do exactly what `unset -f` does, so a fix to either
// reaches both words. Seven times in this tree a second helper has quietly
// missed a fix the first one carried.
func TestTheSecondNamesAreTheSameCode(t *testing.T) {
	for _, tc := range [][2]string{
		{"f(){ echo hi; }\ntypeset -f f\necho st=$?", "f(){ echo hi; }\nfunctions f\necho st=$?"},
		{"f(){ :; }\ng(){ :; }\ntypeset -f", "f(){ :; }\ng(){ :; }\nfunctions"},
		{"typeset -f nosuch\necho st=$?", "functions nosuch\necho st=$?"},
		{"f(){ :; }\nunset -f f\ntypeset -f f\necho st=$?", "f(){ :; }\nunfunction f\nfunctions f\necho st=$?"},
	} {
		first, st1 := run(t, tc[0], withFunctionNames(functionsSem()))
		second, st2 := run(t, tc[1], withFunctionNames(functionsSem()))
		if first != second || st1 != st2 {
			t.Errorf("%q gave %q/%d, %q gave %q/%d — one implementation, two names",
				tc[0], first, st1, tc[1], second, st2)
		}
	}
}

// TestTheComplaintFollowsTheInvokedName, which is what makes the shared
// implementation safe to give a second name: nothing in it spells either
// word, so the sentence a script reads names the builtin it actually ran.
func TestTheComplaintFollowsTheInvokedName(t *testing.T) {
	sem := functionsSem()
	sem.UnsetFunctionReportsMissing = Yes
	dg := Diagnostics{
		UnsetFunctionNotFound: "no such element: %[1]s",
		UnsetNoOperands:       "%[1]s: not enough arguments",
	}
	setup := func(r *Runner) {
		withFunctionNames(sem)(r)
		r.Diagnostics = &dg
	}
	for _, tc := range []struct{ src, want string }{
		// The name is a verb of the wording only where the dialect puts it
		// there. Where it does — this field — both words say their own.
		{"unset", "unset: not enough arguments"},
		{"unfunction", "unfunction: not enough arguments"},
		// And where it does not, the one sentence serves both, which is the
		// half that proves there is no second wording to drift.
		{"unset -f nosuch", "no such element: nosuch"},
		{"unfunction nosuch", "no such element: nosuch"},
	} {
		out, _ := run(t, tc.src+`; echo "st=$?"`, setup)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: out %q, want %q in it", tc.src, out, tc.want)
		}
		if !strings.Contains(out, "st=1\n") {
			t.Errorf("%s: out %q, want status 1", tc.src, out)
		}
	}
}

// TestTheRenamedBuiltinsLettersAreNotTheirOwn. `functions` is `typeset -f`
// and `-f` is not one of its letters; `unfunction` is `unset -f` and takes
// neither `-f` nor `-v`. A shell that reused the renamed builtin's set would
// take both and answer a listing where the real one refuses.
func TestTheRenamedBuiltinsLettersAreNotTheirOwn(t *testing.T) {
	for _, src := range []string{"functions -f", "functions -a", "unfunction -f f", "unfunction -v f"} {
		out, _ := run(t, "f(){ :; }\n"+src+"\necho \"st=$?\"", withFunctionNames(functionsSem()))
		if strings.Contains(out, "f () ") {
			t.Errorf("%s: out %q, want no listing from a letter it does not take", src, out)
		}
		if strings.Contains(out, "st=0\n") {
			t.Errorf("%s: out %q, want the letter refused", src, out)
		}
	}
	// And a letter each really does take is not refused: `-m` is the one
	// implemented letter of either word.
	out, _ := run(t, "fa(){ :; }\nfunctions -m 'f*'\necho \"st=$?\"", withFunctionNames(functionsSem()))
	if !strings.Contains(out, "st=0\n") {
		t.Errorf("out %q, want the implemented letter taken", out)
	}
}

// TestMatchingIsAListingInOneWordAndARemovalInTheOther, including the status
// they deliberately disagree about when nothing matched.
func TestMatchingIsAListingInOneWordAndARemovalInTheOther(t *testing.T) {
	defs := "fa(){ :; }\nfb(){ :; }\ngc(){ :; }\n"
	out, _ := run(t, defs+"functions -m 'f*'\necho \"st=$?\"", withFunctionNames(functionsSem()))
	if strings.Count(out, " () ") != 2 {
		t.Errorf("out %q, want the two matching names listed", out)
	}
	if strings.Contains(out, "gc") || !strings.Contains(out, "st=0\n") {
		t.Errorf("out %q, want only the matches, at 0", out)
	}
	// A name two patterns reach is said once per pattern, which is the
	// measured order: patterns outside, names inside.
	out, _ = run(t, "fa(){ :; }\nfunctions -m 'f*' 'fa'", withFunctionNames(functionsSem()))
	if got := strings.Count(out, "fa"); got != 2 {
		t.Errorf("out %q, want the name said once per pattern, got %d", out, got)
	}
	// No pattern at all is the whole listing rather than nothing.
	out, _ = run(t, defs+"functions -m", withFunctionNames(functionsSem()))
	if !strings.Contains(out, "gc") {
		t.Errorf("out %q, want the bare listing", out)
	}
	out, _ = run(t, defs+"unfunction -m 'f*'\necho \"st=$?\"\nfunctions", withFunctionNames(functionsSem()))
	if strings.Contains(out, "fa") || strings.Contains(out, "fb") || !strings.Contains(out, "gc") {
		t.Errorf("out %q, want only the matches removed", out)
	}
	if !strings.Contains(out, "st=0\n") {
		t.Errorf("out %q, want 0 where something matched", out)
	}
	// Nothing matched: the removal reports 1 where the listing reported 0.
	out, _ = run(t, defs+"unfunction -m 'zz*'\necho \"st=$?\"", withFunctionNames(functionsSem()))
	if !strings.Contains(out, "st=1\n") {
		t.Errorf("out %q, want 1 where nothing matched", out)
	}
	out, _ = run(t, defs+"functions -m 'zz*'\necho \"st=$?\"", withFunctionNames(functionsSem()))
	if !strings.Contains(out, "st=0\n") {
		t.Errorf("out %q, want the listing still 0 with nothing matched", out)
	}
}

// TestTheMatchingLetterPicksTheNamespaceByF — `unset -f -m` is the function
// table and `unset -m` is the parameter table. The letter used to be read
// before `-f` was, so the first spelling removed variables and left every
// function it named standing, in silence at 0.
func TestTheMatchingLetterPicksTheNamespaceByF(t *testing.T) {
	src := "fa(){ :; }\nfa=keep\n%s\necho \"[${fa-gone}]\"\ntypeset -f\n"
	out, _ := run(t, strings.Replace(src, "%s", "unset -f -m 'f*'", 1), withFunctionNames(functionsSem()))
	if !strings.Contains(out, "[keep]") {
		t.Errorf("out %q, want the parameter untouched by the function removal", out)
	}
	if strings.Contains(out, "fa () ") {
		t.Errorf("out %q, want the function removed", out)
	}
	out, _ = run(t, strings.Replace(src, "%s", "unset -m 'f*'", 1), withFunctionNames(functionsSem()))
	if !strings.Contains(out, "[gone]") {
		t.Errorf("out %q, want the parameter removed", out)
	}
	if !strings.Contains(out, "fa () ") {
		t.Errorf("out %q, want the function untouched by the parameter removal", out)
	}
}
