// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A declaration whose *store* refuses the element it names is fatal either
// way and leaves **0** behind in the dialect that answers so.
//
// The refusal is fatal on both routes and its sentence is byte-identical on
// both, so a probe that read the diagnostic cannot see this at all; the only
// observable is the status, which is what #1770 was filed from.
//
// The rows below are the refusal **alone at the top level**, where a script
// file reports 1 whatever the refusal left — that is the shell's own exit and
// not the number, which is the reading #1770 took for a rule about the route
// and #3504 disproved. What raises the 0 back to 1 is
// Runner.refusalZeroIsRaisedBackToOne, whose three places are pinned in
// TestWhereARefusalsZeroIsRaisedBackToOne; a subshell in a script file leaves
// the 0 standing.
func TestAStoreRefusalOfADeclaredElementLeavesZeroAtTheTopLevel(t *testing.T) {
	for _, tc := range []struct {
		why    string
		answer Answer
		route  Route
		want   int
	}{
		{"answered yes, from a command string: 0", Yes, RouteCommandString, 0},
		{"answered yes, from a script file: the file's own exit, 1", Yes, RouteScriptFile, 1},
		{"answered no, from a command string: 1 like every other refusal", No, RouteCommandString, 1},
		{"answered no, from a script file: 1", No, RouteScriptFile, 1},
	} {
		out, errs, st := declaredElementRun(t,
			"a=(x y)\ntypeset a[-5]=v\necho after\n", tc.answer, tc.route)
		if st != tc.want {
			t.Errorf("%s: status %d, want %d", tc.why, st, tc.want)
		}
		// Fatal on every row: the word after it never runs. A 0 that came
		// from the shell carrying on and succeeding would be a different
		// answer wearing the same number, and this is what tells them apart.
		if out != "" {
			t.Errorf("%s: output %q, want the refusal to have ended the shell", tc.why, out)
		}
		if errs == "" {
			t.Errorf("%s: nothing was reported, so this row is not the refusal it names", tc.why)
		}
	}
}

// The zero is the *store's* refusal and not the declaration's, which is what
// makes the field one seam rather than a rule about bad subscripts or about a
// declaration ending the shell.
//
// Both controls end the shell under the same answer and the same route, and
// neither may take the zero: the bare assignment reaches the identical store
// complaint by a route that is not a declaration at all, and a declaration
// refused for the name being frozen is refused before the store is reached.
func TestOnlyTheStoresOwnRefusalTakesTheZero(t *testing.T) {
	for _, tc := range []struct{ why, src string }{
		{"the same refusal from a bare assignment", "a=(x y)\na[-5]=v\necho after\n"},
		{"a declaration refused before the store", "typeset -r a=1\ntypeset a[0]=v\necho after\n"},
	} {
		out, errs, st := declaredElementRun(t, tc.src, Yes, RouteCommandString)
		if errs == "" {
			t.Fatalf("%s: nothing was reported, so this row is not the refusal it names", tc.why)
		}
		if out != "" {
			t.Fatalf("%s: output %q, want the refusal to have ended the shell", tc.why, out)
		}
		if st == 0 {
			t.Errorf("%s: status 0 — the declared-element answer has reached a refusal that is not it", tc.why)
		}
	}
}

// A declaration whose element the store *takes* is untouched by any of this:
// the answer moves a refusal's status and must not move a success's.
func TestAnElementTheStoreTakesIsNotGivenTheZeroRule(t *testing.T) {
	out, errs, st := declaredElementRun(t,
		"a=(x y)\ntypeset a[1]=v\necho \"[${a[1]}]\"\n", Yes, RouteCommandString)
	if errs != "" {
		t.Fatalf("stderr %q, want the element taken", errs)
	}
	if out != "[v]\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "[v]\n")
	}
}

func declaredElementRun(t *testing.T, src string, answer Answer, route Route) (string, string, int) {
	t.Helper()
	sem := permissive()
	// The axes on the way to the store, answered so the run reaches the
	// refusal rather than stopping at an unanswered question.
	sem.ArrayBaseIsZero = Yes
	sem.TypesetTakesASubscript = Yes
	sem.NegativeSubscriptPastTheStartInserts = No
	sem.FatalErrorStatusIsOne = Yes
	sem.StoreRefusalOfADeclaredElementLeavesZero = answer
	return declaredElementRunWith(t, src, sem, route)
}

func declaredElementRunWith(t *testing.T, src string, sem Semantics, route Route) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Route: route,
		Diagnostics: &Diagnostics{
			Location:          LocationColonLine,
			BadArraySubscript: "%[1]s[%[2]s]: bad array subscript",
		},
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}
