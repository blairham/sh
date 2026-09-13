// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// These tests name axes, never shells. Which dialect answers an axis which
// way is dialect/<shell>'s to assert; what is pinned here is that each axis
// is reachable, that both answers do what they say, and that the substrate
// refuses when no dialect answered.

// pseudoSem answers just enough for a test about one pseudo-condition: the
// conditions exist, a trap body is read the way a script is, and the traps
// stay where they were set until a test says otherwise.
func pseudoSem() Semantics {
	s := CoreSemantics()
	s.TrapHasErrCondition = Yes
	s.TrapHasDebugCondition = Yes
	s.TrapHasReturnCondition = Yes
	s.TrapBodyRunsWhatParsed = Yes
	s.ErrTrapRunsInsideFunctions = No
	s.DebugTrapRunsInsideCalls = No
	s.ErrTrapRunsInSubshells = No
	s.DebugTrapRunsInSubshells = No
	// One head per command and no more, which is what these tests count.
	// The extra head one column writes as a call enters a body is its own
	// axis and its own suite; see debughead_test.go.
	s.DebugTrapFiresOnEnteringAFunction = No
	return s
}

// pseudoRun runs src in a directory of its own, so a test that sources a
// file can write one first.
func pseudoRun(t *testing.T, dir, src string, sem Semantics) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// TestPseudoConditionsAreTheDialectsToHave: each name is taken where the
// axis says yes, refused as the unknown word it is where the axis says no,
// and refused as unanswered by the core.
func TestPseudoConditionsAreTheDialectsToHave(t *testing.T) {
	dg := Diagnostics{TrapBadSignal: "trap: %[1]s: bad trap"}
	for _, cond := range []string{"ERR", "DEBUG", "RETURN"} {
		t.Run(cond, func(t *testing.T) {
			// The action is `:` because a DEBUG action would run before the
			// next command; acceptance is what this test is about, and the
			// firing rules have tests of their own.
			out, st := run(t, `trap ':' `+cond+`; echo "st=$?"`, func(r *Runner) {
				s := pseudoSem()
				r.Semantics, r.Diagnostics = &s, &dg
			})
			if out != "st=0\n" || st != 0 {
				t.Errorf("axis yes: got %q/%d, want accepted silently", out, st)
			}

			no := pseudoSem()
			no.TrapHasErrCondition, no.TrapHasDebugCondition, no.TrapHasReturnCondition = No, No, No
			out, st = run(t, `trap 'echo x' `+cond+`; echo "st=$?"`, func(r *Runner) {
				r.Semantics, r.Diagnostics = &no, &dg
			})
			// The same complaint any word that names no signal gets, status
			// 1, and the script carries on — measured on the one shell that
			// refuses all three and on the two that refuse RETURN.
			if want := "sh: trap: " + cond + ": bad trap\nst=1\n"; out != want || st != 0 {
				t.Errorf("axis no: got %q/%d, want %q/0", out, st, want)
			}

			out, _ = run(t, `trap 'echo x' `+cond, withSem(CoreSemantics()))
			if !strings.Contains(out, "no dialect was chosen") {
				t.Errorf("core: got %q, want a refusal naming the axis", out)
			}
		})
	}
}

// TestErrTrapFiresWhereErrexitWouldJudge: the trap runs after a failing
// command, with or without `set -e`, and not in the places `set -e` would
// not look — a condition, a chain operand, a negation.
func TestErrTrapFiresWhereErrexitWouldJudge(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantOut   string
		wantSt    int
	}{
		{
			"a failing command fires it and the script goes on",
			`trap 'echo E' ERR; false; echo done`, "E\ndone\n", 0,
		},
		{
			"each failure fires it again",
			`trap 'echo E' ERR; false; true; false; echo done`, "E\nE\ndone\n", 0,
		},
		{
			"the action sees the failing status and leaves it in place",
			`trap 'echo "saw=$?"' ERR; f() { return 7; }; f; echo "after=$?"`, "saw=7\nafter=7\n", 0,
		},
		{
			"an if condition fires nothing",
			`trap 'echo E' ERR; if false; then :; fi; echo done`, "done\n", 0,
		},
		{
			"a chain operand fires nothing",
			`trap 'echo E' ERR; false && :; echo done`, "done\n", 0,
		},
		{
			"a negation fires nothing",
			`trap 'echo E' ERR; ! false; echo done`, "done\n", 0,
		},
		{
			"with errexit the action runs and the script then stops",
			`set -e; trap 'echo E' ERR; false; echo no`, "E\n", 1,
		},
		{
			"a failure inside the action does not fire it again",
			`trap 'echo E; false' ERR; false; echo done`, "E\ndone\n", 0,
		},
		{
			"an exit in the action wins",
			`trap 'exit 9' ERR; false; echo no`, "", 9,
		},
		{
			"an empty action ignores rather than fires",
			`trap '' ERR; false; echo done`, "done\n", 0,
		},
		{
			"trap - puts the default back",
			`trap 'echo E' ERR; trap - ERR; false; echo done`, "done\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, withSem(pseudoSem()))
			if got != tc.wantOut || st != tc.wantSt {
				t.Errorf("got %q/%d, want %q/%d", got, st, tc.wantOut, tc.wantSt)
			}
		})
	}
}

// TestDebugTrapFiresBeforeEachSimpleCommand, and its action cannot change
// the `$?` the command it precedes will see.
func TestDebugTrapFiresBeforeEachSimpleCommand(t *testing.T) {
	sem := pseudoSem()
	got, st := run(t, `trap 'echo D' DEBUG; echo a; echo b`, withSem(sem))
	if want := "D\na\nD\nb\n"; got != want || st != 0 {
		t.Errorf("got %q/%d, want %q/0", got, st, want)
	}
	// The action sees the previous command's status and does not disturb it.
	got, _ = run(t, `trap 'echo "saw=$?"' DEBUG; f() { return 5; }; f; true`, withSem(sem))
	if want := "saw=0\nsaw=5\n"; got != want {
		t.Errorf("status seen: got %q, want %q", got, want)
	}
	got, _ = run(t, `trap 'true' DEBUG; false; echo "st=$?"`, withSem(sem))
	if want := "st=1\n"; got != want {
		t.Errorf("status kept: got %q, want %q", got, want)
	}
}

// TestPseudoTrapsInsideFunctions: whether ERR and DEBUG follow the script
// into a function the trap was not set in is each trap's own axis, and the
// suppression is per frame — a trap set inside a function still fires there
// and at the top level afterwards, and not inside a sibling's body.
func TestPseudoTrapsInsideFunctions(t *testing.T) {
	errSrc := `trap 'echo E' ERR; f() { false; echo body; }; f; echo done`
	debugSrc := `trap 'echo D' DEBUG; f() { echo in; }; f; echo out`

	sem := pseudoSem()
	sem.ErrTrapRunsInsideFunctions = No
	sem.DebugTrapRunsInsideCalls = No
	if got, _ := run(t, errSrc, withSem(sem)); got != "body\ndone\n" {
		t.Errorf("ERR bounded: got %q, want no E", got)
	}
	if got, _ := run(t, debugSrc, withSem(sem)); got != "D\nin\nD\nout\n" {
		t.Errorf("DEBUG bounded: got %q, want the call fired and the body not", got)
	}
	// Set inside a function: fires there, fires at the top level after,
	// and does not follow into a sibling's body — though the sibling's
	// call is a command outside it and still fires DEBUG.
	setInside := `f() { trap 'echo D' DEBUG; echo a; }; f; echo top; g() { echo ing; }; g`
	if got, _ := run(t, setInside, withSem(sem)); got != "D\na\nD\ntop\nD\ning\n" {
		t.Errorf("DEBUG set inside: got %q", got)
	}

	sem.ErrTrapRunsInsideFunctions = Yes
	sem.DebugTrapRunsInsideCalls = Yes
	if got, _ := run(t, errSrc, withSem(sem)); got != "E\nbody\ndone\n" {
		t.Errorf("ERR follows: got %q, want E from inside", got)
	}
	if got, _ := run(t, debugSrc, withSem(sem)); got != "D\nD\nin\nD\nout\n" {
		t.Errorf("DEBUG follows: got %q, want the body fired too", got)
	}
}

// TestPseudoTrapsInSubshells: two axes, because the panel groups them
// differently — one dialect carries DEBUG into a command substitution and
// not ERR.
func TestPseudoTrapsInSubshells(t *testing.T) {
	errSub := `trap 'echo E' ERR; x=$(false; echo hi); echo "got=[$x]"`
	debugSub := `trap 'echo D' DEBUG; (echo sub); echo done`

	sem := pseudoSem()
	sem.ErrTrapRunsInSubshells = No
	sem.DebugTrapRunsInSubshells = No
	if got, _ := run(t, errSub, withSem(sem)); got != "got=[hi]\n" {
		t.Errorf("ERR out of subshells: got %q", got)
	}
	if got, _ := run(t, debugSub, withSem(sem)); got != "sub\nD\ndone\n" {
		t.Errorf("DEBUG out of subshells: got %q", got)
	}

	sem.ErrTrapRunsInSubshells = Yes
	sem.DebugTrapRunsInSubshells = Yes
	if got, _ := run(t, errSub, withSem(sem)); got != "got=[E\nhi]\n" {
		t.Errorf("ERR into subshells: got %q, want the E captured", got)
	}
	if got, _ := run(t, debugSub, withSem(sem)); got != "D\nsub\nD\ndone\n" {
		t.Errorf("DEBUG into subshells: got %q", got)
	}
}

// TestReturnTrapFiresForSourcesAndForTheFrameThatSetIt: a sourced file
// fires it wherever it was set; a function fires it only when its own body
// did the setting — not a caller's trap, and not a sibling entered after
// the setter returned.
func TestReturnTrapFiresForSourcesAndForTheFrameThatSetIt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.sh"), []byte("echo insource\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sem := pseudoSem()
	sem.DotPassesArguments = No

	if got, _ := pseudoRun(t, dir, `trap 'echo R' RETURN; . ./lib.sh; echo after`, sem); got != "insource\nR\nafter\n" {
		t.Errorf("source: got %q, want R after the file", got)
	}
	if got, _ := pseudoRun(t, dir, `trap 'echo R' RETURN; f() { :; }; f; echo after`, sem); got != "after\n" {
		t.Errorf("function without its own setting: got %q, want no R", got)
	}
	src := `f() { trap 'echo R' RETURN; :; }; f; echo mid; g() { :; }; g; echo end`
	if got, _ := pseudoRun(t, dir, src, sem); got != "R\nmid\nend\n" {
		t.Errorf("set inside: got %q, want R for the setter only", got)
	}
	// An explicit return fires it too, and the action sees the status
	// `return` began with rather than the argument the caller gets.
	src = `f() { trap 'echo "R=$?"' RETURN; return 3; }; f; echo "st=$?"`
	if got, _ := pseudoRun(t, dir, src, sem); got != "R=0\nst=3\n" {
		t.Errorf("explicit return: got %q", got)
	}
}

// TestPseudoTrapsAreListedAfterTheSignals, bare of any SIG prefix, and a
// single-word `trap ERR` puts one back the way `trap - ERR` does.
func TestPseudoTrapsAreListedAfterTheSignals(t *testing.T) {
	sem := pseudoSem()
	sem.TrapQuoting = ListingQuoteAlwaysEscaped
	sem.TrapOneArgumentIsACondition = Yes
	dg := Diagnostics{TrapPrintsSignalPrefix: "SIG"}
	// The actions are `: n` because the DEBUG one runs before each later
	// command — an action that was a bare letter would be run as one.
	out, _ := run(t, "trap ': 1' INT; trap ': 2' ERR; trap ': 3' DEBUG; trap", func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	want := "trap -- ': 1' SIGINT\ntrap -- ': 3' DEBUG\ntrap -- ': 2' ERR\n"
	if out != want {
		t.Errorf("listing: got %q, want %q", out, want)
	}
	out, _ = run(t, `trap 'echo E' ERR; trap ERR; false; echo done`, func(r *Runner) {
		r.Semantics = &sem
	})
	if out != "done\n" {
		t.Errorf("single-word reset: got %q, want the trap gone", out)
	}
}
