// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.FunctionLocalTraps, asked of the substrate rather than of a
// shell: what the axis *does*, both ways, with the dialect that spells it as
// an option left to dialect/zsh.
//
// Every handler prints something distinct on purpose. A probe whose inner and
// outer traps echo the same word cannot tell "put back" from "left in place",
// and a probe with no outer trap cannot tell "put the outer one back" from
// "cleared everything" — so the rows below always have both, except the two
// that are *about* there being no outer trap and say so.

// localTrapsSem is the base every row runs on: this axis at the answer being
// asked about, and how `trap` spells a value answered so a listing is a
// listing rather than a refusal.
func localTrapsSem(locality TrapLocality) Semantics {
	s := permissive()
	s.FunctionLocalTraps = locality
	s.TrapQuoting = ListingQuoteAlwaysEscaped
	return s
}

func localTrapsRun(t *testing.T, locality TrapLocality, src string) (string, int) {
	t.Helper()
	s := localTrapsSem(locality)
	return run(t, src, withSem(s))
}

// Both sides of the axis on one snippet: the answer that scopes puts the
// caller's handler back, and the answer that does not leaves the function's
// installed. Asserting one side alone would assert a default.
func TestFunctionLocalTrapsHasTwoSides(t *testing.T) {
	const src = `trap 'echo outer' USR1; f() { trap 'echo inner' USR1; }; f; trap`
	if out, st := localTrapsRun(t, TrapsGoBackAtTheReturn, src); out != "trap -- 'echo outer' USR1\n" || st != 0 {
		t.Errorf("scoped: got %q/%d, want the caller's trap back", out, st)
	}
	if out, st := localTrapsRun(t, TrapsSurviveTheFunction, src); out != "trap -- 'echo inner' USR1\n" || st != 0 {
		t.Errorf("unscoped: got %q/%d, want the function's trap standing", out, st)
	}
	// And an unanswered vector reads as the answer every shell shares rather
	// than refusing: there is no disagreement here to refuse over.
	if out, st := localTrapsRun(t, TrapLocalityUnspecified, src); out != "trap -- 'echo inner' USR1\n" || st != 0 {
		t.Errorf("unanswered: got %q/%d, want the same as TrapsSurviveTheFunction", out, st)
	}
}

// The save is per condition: a second condition the same body moved while the
// axis said otherwise is left where the body put it. One snippet moving two
// signals under two answers is what says the save is not a whole-table
// snapshot taken at the first modification.
func TestFunctionLocalTrapsSavesPerCondition(t *testing.T) {
	out, st := run(t, `trap 'echo o1' USR1; trap 'echo o2' USR2; `+
		`f() { trap 'echo i1' USR1; unscope; trap 'echo i2' USR2; }; f; trap`,
		func(r *Runner) {
			s := localTrapsSem(TrapsGoBackAtTheReturn)
			r.Semantics = &s
			// A builtin standing in for the option the dialect spells this
			// with, so the axis can move mid-body without this package
			// knowing an option's name.
			r.Register("unscope", func(r *Runner, _ context.Context, _ []string) int {
				s := *r.Semantics
				s.FunctionLocalTraps = TrapsSurviveTheFunction
				r.Semantics = &s
				return 0
			})
		})
	want := "trap -- 'echo o1' USR1\ntrap -- 'echo i2' USR2\n"
	if out != want || st != 0 {
		t.Errorf("got %q/%d, want %q: USR1 restored and USR2 left as the body set it", out, st, want)
	}
}

// First save wins. A body that sets the same condition twice goes back to
// what it displaced, not to what it set first — three distinct handlers, so
// the wrong answer is visible rather than merely equal.
func TestFunctionLocalTrapsKeepsTheFirstSave(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap 'echo outer' USR1; f() { trap 'echo one' USR1; trap 'echo two' USR1; }; f; trap`)
	if out != "trap -- 'echo outer' USR1\n" || st != 0 {
		t.Errorf("got %q/%d, want the caller's trap and not either of the body's", out, st)
	}
}

// A reset is a modification like a set, so `trap -` inside the body is undone
// too and the caller's handler comes back.
func TestFunctionLocalTrapsUndoesAReset(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap 'echo outer' USR1; f() { trap - USR1; trap; echo '(inside)'; }; f; trap`)
	want := "(inside)\ntrap -- 'echo outer' USR1\n"
	if out != want || st != 0 {
		t.Errorf("got %q/%d, want %q: nothing listed inside and the handler back after", out, st, want)
	}
}

// And the one-word spelling of a reset is a reset: three of the four shells
// read `trap COND` as putting that condition back, and it reaches the same
// save as the two-word form — a second entry point that a fix written at one
// of them alone would miss.
func TestFunctionLocalTrapsUndoesAOneWordReset(t *testing.T) {
	s := localTrapsSem(TrapsGoBackAtTheReturn)
	s.TrapOneArgumentIsACondition = Yes
	out, st := run(t, `trap 'echo outer' USR1; f() { trap USR1; trap; echo '(inside)'; }; f; trap`, withSem(s))
	want := "(inside)\ntrap -- 'echo outer' USR1\n"
	if out != want || st != 0 {
		t.Errorf("got %q/%d, want %q", out, st, want)
	}
}

// An ignore is a third state and travels as one: `trap ” USR1` displaced by
// a handler comes back as an ignore rather than as nothing.
func TestFunctionLocalTrapsRestoresAnIgnore(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap '' USR1; f() { trap 'echo inner' USR1; }; f; trap`)
	if out != "trap -- '' USR1\n" || st != 0 {
		t.Errorf("got %q/%d, want the ignore back", out, st)
	}
}

// A pseudo-condition is a condition. ERR, DEBUG and RETURN live in slots of
// their own rather than in the signal table, and the save reaches them the
// same way — asked here of ERR, since no dialect in the panel has both a
// pseudo-condition and this axis, so the corpus cannot ask it.
func TestFunctionLocalTrapsReachAPseudoCondition(t *testing.T) {
	s := localTrapsSem(TrapsGoBackAtTheReturn)
	s.TrapHasErrCondition = Yes
	s.ErrTrapRunsInsideFunctions = No
	s.ErrTrapRunsInSubshells = No
	out, st := run(t, `trap 'echo outer' ERR; f() { trap 'echo inner' ERR; }; f; false`, withSem(s))
	if out != "outer\n" || st != 1 {
		t.Errorf("got %q/%d, want the caller's ERR trap back and firing", out, st)
	}
}

// And what goes back with it is *where* the trap was set, not only its text.
// A pseudo-trap fires for the frame that set it and for nobody else, so an
// action put back under the wrong frame is a handler that is listed and never
// runs — the quiet half of this restore.
//
// The failure is inside the function that set the outer trap, which is the
// only arrangement that tells the two apart: at the top level the frame is
// not consulted, and outside the setter both frames are equally wrong.
func TestFunctionLocalTrapsRestoreWhereAPseudoTrapWasSet(t *testing.T) {
	s := localTrapsSem(TrapsGoBackAtTheReturn)
	s.TrapHasErrCondition = Yes
	s.ErrTrapRunsInsideFunctions = No
	s.ErrTrapRunsInSubshells = No
	out, st := run(t, `inner() { trap 'echo inner' ERR; }; `+
		`outer() { trap 'echo outer' ERR; inner; false; }; outer`, withSem(s))
	if out != "outer\n" || st != 1 {
		t.Errorf("got %q/%d, want the outer trap firing in the frame that set it", out, st)
	}
}

// Each call answers for its own modifications at its own return, so an inner
// call restores while the outer one is still running.
func TestFunctionLocalTrapsRestoresAtEachNestedReturn(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap 'echo outer' USR1; i() { trap 'echo inner' USR1; }; `+
			`o() { trap 'echo middle' USR1; i; trap; echo '(in o)'; }; o; trap`)
	want := "trap -- 'echo middle' USR1\n(in o)\ntrap -- 'echo outer' USR1\n"
	if out != want || st != 0 {
		t.Errorf("got %q/%d, want %q", out, st, want)
	}
}

// A `return` from the middle of the body restores exactly as falling off the
// end does, and the status it asked for still travels.
func TestFunctionLocalTrapsRestoresOnAnEarlyReturn(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap 'echo outer' USR1; f() { trap 'echo inner' USR1; return 3; }; f; echo rc=$?; trap`)
	want := "rc=3\ntrap -- 'echo outer' USR1\n"
	if out != want || st != 0 {
		t.Errorf("got %q/%d, want %q", out, st, want)
	}
}

// A subshell written in the body is not the body. It starts with the parent's
// handled traps back at their defaults, so a save taken there would record
// "nothing was set" for a condition the caller is handling — and the caller's
// trap would then be cleared by a return that never touched it.
func TestFunctionLocalTrapsIgnoreASubshellsOwnChanges(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap 'echo outer' USR1; f() { ( trap 'echo sub' USR1 ); }; f; trap`)
	if out != "trap -- 'echo outer' USR1\n" || st != 0 {
		t.Errorf("got %q/%d, want the caller's trap untouched", out, st)
	}
}

// EXIT is not this question: it has ExitTrapIsFunctionLocal, which is asked
// whatever this axis says. Both answers of that one, under the axis that
// scopes everything else, so the rows say this mechanism keeps its hands off
// rather than happening to agree.
func TestFunctionLocalTrapsLeaveExitAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit Answer
		want string
	}{
		{"exit trap fires at the return", Yes, "body\ninner\nafter\nouter\n"},
		{"exit trap waits for the end", No, "body\nafter\ninner\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := localTrapsSem(TrapsGoBackAtTheReturn)
			s.ExitTrapIsFunctionLocal = tc.exit
			out, st := run(t, `trap 'echo outer' EXIT; f() { trap 'echo inner' EXIT; echo body; }; f; echo after`,
				withSem(s))
			if out != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q", out, st, tc.want)
			}
		})
	}
}

// What comes back is the *disposition*, not the listing. The signal is sent
// for real and the handler that runs is the observation: after the return it
// is the caller's, and inside the body it is the function's.
//
// Deadlined because a trap that never fires is a test that hangs rather than
// one that fails, and a hang costs the package's whole budget — see
// deadline_test.go. Both rows finish in milliseconds when they work.
func TestFunctionLocalTrapsRestoreTheDisposition(t *testing.T) {
	deadline(t, "a signal sent either side of a scoped return", func() {
		out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
			`trap 'echo outer' USR1; f() { trap 'echo inner' USR1; kill -USR1 $$; }; f; `+
				`kill -USR1 $$; echo done`)
		want := "inner\nouter\ndone\n"
		if out != want || st != 0 {
			t.Errorf("got %q/%d, want %q: the body's handler inside and the caller's after", out, st, want)
		}
	})
}

// And with nothing displaced, what comes back is the default action — which
// for USR1 is death. The row that says the restore is not "put the listing
// back": a shell that merely forgot the listing would still have the handler
// arranged and would print `handled`.
func TestFunctionLocalTrapsRestoreTheDefaultAction(t *testing.T) {
	// 128 plus the signal's number, asked of the platform: only the first
	// fifteen signals are fixed by the standard, and USR1 is 30 on Darwin
	// and 10 on Linux — a literal 158 here is a fact about one machine.
	want := 128 + int(syscall.SIGUSR1)
	deadline(t, "a signal sent after a scoped return with nothing to go back to", func() {
		out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
			`f() { trap 'echo handled' USR1; }; f; echo before; kill -USR1 $$; echo unreached`)
		if !strings.HasPrefix(out, "before\n") || strings.Contains(out, "handled") ||
			strings.Contains(out, "unreached") || st != want {
			t.Errorf("got %q/%d, want the shell killed by USR1 at %d with no handler run", out, st, want)
		}
	})
}

// A trap set where there is no call to return from stays set. The top level
// is the row, and a sourced file is the same row written the other way: it
// pushes a frame and not a scope, so a trap it sets at *its* top level is the
// script's.
func TestFunctionLocalTrapsLeaveTheTopLevelAlone(t *testing.T) {
	out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
		`trap 'echo outer' USR1; trap 'echo second' USR1; trap`)
	if out != "trap -- 'echo second' USR1\n" || st != 0 {
		t.Errorf("got %q/%d, want the second trap standing", out, st)
	}
}

// The arrival that lands on the body's *last* command is handled inside the
// call, by the trap that was installed when it arrived — not by the one the
// return is about to put back.
//
// stmt drains between commands, which leaves the last command of a body with
// nobody to drain after it. Unanimous across the panel and so not an axis:
// bash 3.2, bash 5.3, dash, ksh93 and zsh all print `inner` here. It was
// invisible until a trap could be put back at a return, because with the
// handler unchanged either side of the boundary a late run only moved the
// output.
func TestAnArrivalOnTheLastCommandOfABodyIsHandledInside(t *testing.T) {
	deadline(t, "a signal raised by the last command of a function body", func() {
		out, st := localTrapsRun(t, TrapsGoBackAtTheReturn,
			`trap 'echo outer' USR1; f() { trap 'echo inner' USR1; kill -USR1 $$; }; f; echo after`)
		want := "inner\nafter\n"
		if out != want || st != 0 {
			t.Errorf("got %q/%d, want %q", out, st, want)
		}
	})
}
