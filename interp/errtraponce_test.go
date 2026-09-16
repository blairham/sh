// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// One failure, one firing — and where a second firing is real, what makes it
// real.
//
// The bug these pin (#2793) was that checkErrExit is asked once per
// *statement* and a compound is a statement: a group, a loop, an `if` and a
// `case` report the status their last command left, so the same failure was
// judged again on the way out and a handler ran twice for one event — three
// times two compounds deep, four times three deep. No column in the panel
// does that.
//
// The firings that are not the bug are pinned here too, because a fix that
// suppressed them would be a worse one: each pass of a loop is its own
// failure, a subshell really is a second boundary, and in two of the four
// columns a *call* is a second boundary as well.

// errSem is the ERR condition with the trap kept out of functions and
// subshells, so what fires in a test is the failure itself plus whatever the
// axis under test adds.
func errSem() Semantics {
	s := trapSem()
	s.TrapHasErrCondition = Yes
	s.ErrTrapRunsInsideFunctions = No
	s.ErrTrapRunsInSubshells = No
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapAlwaysRefires
	return s
}

func TestOneFailureInsideACompoundFiresTheErrTrapOnce(t *testing.T) {
	for _, refiring := range everyRefiring {
		t.Run(refiring.String(), func(t *testing.T) {
			s := errSem()
			s.ErrTrapRefiresForTheCommandItFiredInside = refiring
			oneFailureFiresOnce(t, s)
		})
	}
}

// everyRefiring is every answer to the refiring axis that *is* an answer. A
// compound is not that axis, and each pass of a loop is not either, so the
// rows under them read the same in every column — which is only worth
// asserting if the assertion is *made* in every column. It was not: with the
// axis left at "always refires", a simple command puts the record back on
// its way out and does the statement head's job for it, so a test that ran
// under that answer alone could not see the head's clear go missing. The
// missing clear is `for i in 1 2; do false; done` writing one E.
var everyRefiring = []ErrTrapRefiring{
	ErrTrapFiresOnceForTheFailure,
	ErrTrapRefiresWhereItWasSetFirst,
	ErrTrapAlwaysRefires,
}

func oneFailureFiresOnce(t *testing.T, s Semantics) {
	t.Helper()
	for _, tc := range []struct{ name, src string }{
		{"a group", `trap 'echo E' ERR; { true; false; }; echo done`},
		{"a loop body", `trap 'echo E' ERR; for i in 1; do false; done; echo done`},
		{"an if body", `trap 'echo E' ERR; if true; then false; fi; echo done`},
		{"a case arm", `trap 'echo E' ERR; case x in x) false;; esac; echo done`},
		{"a while body", `trap 'echo E' ERR; while true; do false; break; done; echo done`},
		// The nesting is the half a one-level fix would leave broken:
		// suppressing the second firing at one depth still doubles at the
		// next. Two levels wrote three E lines and three levels wrote four.
		{"a group in a group", `trap 'echo E' ERR; { { false; }; }; echo done`},
		{"a loop in an if in a group", `trap 'echo E' ERR; { if true; then for i in 1; do false; done; fi; }; echo done`},
		{"four deep", `trap 'echo E' ERR; { { if true; then for i in 1; do false; done; fi; }; }; echo done`},
	} {
		if got, _ := run(t, tc.src, withSem(s)); got != "E\ndone\n" {
			t.Errorf("%s: got %q, want one E", tc.name, got)
		}
	}
}

func TestEachFailureFiresTheErrTrapOfItsOwn(t *testing.T) {
	// The other side of the rule, and the reason it is not "once per
	// compound": every column that has the condition writes two E lines for
	// a loop whose body fails twice.
	//
	// In every column, so under every answer to the refiring axis — which
	// is what makes this see the clear at the head of a statement. Under
	// "always refires" alone it could not: a simple command puts the record
	// back on its way out, so the second `false` fired whether or not its
	// statement had cleared anything.
	for _, refiring := range everyRefiring {
		s := errSem()
		s.ErrTrapRefiresForTheCommandItFiredInside = refiring
		for _, tc := range []struct{ name, src string }{
			{"two passes", `trap 'echo E' ERR; for i in 1 2; do false; done; echo done`},
			{"two statements", `trap 'echo E' ERR; { false; false; }; echo done`},
			{"two compounds", `trap 'echo E' ERR; { false; }; { false; }; echo done`},
			{"two failures at the top", `trap 'echo E' ERR; false; false; echo done`},
		} {
			if got, _ := run(t, tc.src, withSem(s)); got != "E\nE\ndone\n" {
				t.Errorf("%s under %s: got %q, want two E", tc.name, refiring, got)
			}
		}
	}
}

func TestASubshellStillFiresOnBothSides(t *testing.T) {
	// A subshell is a second boundary rather than a second reading of the
	// same one: the child judges the failure and the parent judges the
	// failing subshell *command*. Measured on zsh 5.9.2, which is the
	// column that carries the trap into the child: `trap 'echo E' ERR;
	// (false)` writes two.
	//
	// It needs nothing in the fix because the child is a copy — the record
	// of the firing is made on the copy — and this is here so that a later
	// change cannot quietly make the copy share it.
	s := errSem()
	s.ErrTrapRunsInSubshells = Yes
	if got, _ := run(t, `trap 'echo E' ERR; ( false ); echo done`, withSem(s)); got != "E\nE\ndone\n" {
		t.Errorf("got %q, want both sides", got)
	}
	// And the group around it is still not a third.
	if got, _ := run(t, `trap 'echo E' ERR; { ( false ); }; echo done`, withSem(s)); got != "E\nE\ndone\n" {
		t.Errorf("group around a subshell: got %q, want both sides and no more", got)
	}
}

func TestTheCallIsASecondFiringByItsAxis(t *testing.T) {
	// With the trap carried into the call, the failure inside the body
	// fires; whether the call itself fires again is the axis. ksh93 writes
	// four E lines for a failure three calls deep and zsh writes one.
	s := errSem()
	s.ErrTrapRunsInsideFunctions = Yes
	const deep = `trap 'echo E' ERR; f() { g; }; g() { h; }; h() { false; }; f; echo done`
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapAlwaysRefires
	if got, _ := run(t, `trap 'echo E' ERR; g() { false; }; g; echo done`, withSem(s)); got != "E\nE\ndone\n" {
		t.Errorf("always refires: got %q, want the failure and the call", got)
	}
	if got, _ := run(t, deep, withSem(s)); got != "E\nE\nE\nE\ndone\n" {
		t.Errorf("always refires, three deep: got %q, want four", got)
	}
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapFiresOnceForTheFailure
	if got, _ := run(t, `trap 'echo E' ERR; g() { false; }; g; echo done`, withSem(s)); got != "E\ndone\n" {
		t.Errorf("fires once: got %q, want one E", got)
	}
	if got, _ := run(t, deep, withSem(s)); got != "E\ndone\n" {
		t.Errorf("fires once, three deep: got %q, want one E", got)
	}
}

func TestARefiringThatNeedsATrapSetBeforeTheCommand(t *testing.T) {
	// bash's answer, and it is one answer rather than two: the call refires
	// only where an ERR trap was in force when the call began. Measured
	// from a script file with the action printing $LINENO — with the trap
	// set at the top and `set -E` carrying it in, bash 5.3.15 fires at the
	// body's line and again at the call's; with the trap set for the first
	// time inside the function it fires at the body's line alone.
	s := errSem()
	s.ErrTrapRunsInsideFunctions = Yes
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapRefiresWhereItWasSetFirst
	if got, _ := run(t, `trap 'echo E' ERR; g() { false; }; g; echo done`, withSem(s)); got != "E\nE\ndone\n" {
		t.Errorf("a trap set before the call: got %q, want two", got)
	}
	if got, _ := run(t, `g() { trap 'echo I' ERR; false; }; g; echo done`, withSem(s)); got != "I\ndone\n" {
		t.Errorf("a trap set inside the call: got %q, want one", got)
	}
	// And the same script under the other answer, which is what makes the
	// row a measurement of the axis rather than of the shape: ksh93 and ash
	// both write two here.
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapAlwaysRefires
	if got, _ := run(t, `g() { trap 'echo I' ERR; false; }; g; echo done`, withSem(s)); got != "I\nI\ndone\n" {
		t.Errorf("a trap set inside the call, always refiring: got %q, want two", got)
	}
}

func TestSourcingAndEvalRefireLikeACall(t *testing.T) {
	// The axis is about a command that *ran* the failure, not about
	// functions: `. ./lib.sh` over a file holding `false` writes two E lines
	// in bash 5.3.15, bash 3.2 and ksh93 and one in zsh, and `eval false`
	// splits the same way. A sourced file never bounded the ERR trap in any
	// column, so the failure inside fires wherever the trap is set.
	s := errSem()
	for _, tc := range []struct{ name, src string }{
		{"a sourced file", `trap 'echo E' ERR; echo false > lib.sh; . ./lib.sh; echo done`},
		{"eval", `trap 'echo E' ERR; eval false; echo done`},
	} {
		s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapAlwaysRefires
		if got, _ := run(t, tc.src, withSem(s)); got != "E\nE\ndone\n" {
			t.Errorf("%s, always refiring: got %q, want two", tc.name, got)
		}
		s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapFiresOnceForTheFailure
		if got, _ := run(t, tc.src, withSem(s)); got != "E\ndone\n" {
			t.Errorf("%s, firing once: got %q, want one", tc.name, got)
		}
	}
}

func TestAReturnIsJudgedInsideTheFrameTheCallIsNot(t *testing.T) {
	// Where the call is not a firing site, the body's own status is judged
	// in the frame instead: the `return` sets control flow, so the statement
	// holding it is not judged, and the call that reports its status is not
	// judged either. The count does not tell the two apart — one firing
	// either way — so the rows that matter here are the ones that read the
	// frame. zsh 5.9.2 writes one E for `g() { return 1; }; g` with
	// `${funcstack[*]}` reading `g`.
	s := errSem()
	s.ErrTrapRunsInsideFunctions = Yes
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapFiresOnceForTheFailure
	if got, _ := run(t, `trap 'echo E' ERR; g() { return 1; }; g; echo done`, withSem(s)); got != "E\ndone\n" {
		t.Errorf("a bare return: got %q, want one E", got)
	}
	// Two failures rather than one doubled: the `false` is announced where
	// it happens, and the `return 1` is a second status the body reports.
	if got, _ := run(t, `trap 'echo E' ERR; g() { false; return 1; }; g; echo done`, withSem(s)); got != "E\nE\ndone\n" {
		t.Errorf("a failure and a return: got %q, want two E", got)
	}
	// A body that returns 0 is not judged at all, whatever it did on the
	// way: the status the call reports is a success.
	if got, _ := run(t, `trap 'echo E' ERR; g() { false; return 0; }; g; echo done`, withSem(s)); got != "E\ndone\n" {
		t.Errorf("a return of 0: got %q, want the failure alone", got)
	}
	// And the frame judgment is subject to the same exemption everything
	// else is: a call whose status is being *tested* fires nothing.
	if got, _ := run(t, `trap 'echo E' ERR; g() { return 1; }; if g; then echo t; else echo f; fi`, withSem(s)); got != "f\n" {
		t.Errorf("a tested call: got %q, want no E", got)
	}
	// *Inside* the frame, which is the whole of what this adds and the only
	// thing that can see it: the count is the same either way, because a
	// body that returns non-zero leaves the call reporting the same status
	// and the call would be judged instead. What differs is what the action
	// can read. Measured on zsh 5.9.2, which is the column that judges the
	// frame: with the body declaring `local v=in` the action prints `in`
	// where the caller's is `out`, and `${funcstack[*]}` reads `g`. bash,
	// which judges the call, prints `out` and an empty stack.
	if got, _ := run(t, `trap 'echo v=$v' ERR; g() { local v=in; return 1; }; v=out; g; echo done`, withSem(s)); got != "v=in\ndone\n" {
		t.Errorf("the action should see the frame's locals: got %q, want v=in", got)
	}
	// The positional parameters go with them.
	if got, _ := run(t, `trap 'echo p=$1' ERR; g() { return 1; }; set -- outer; g inner; echo done`, withSem(s)); got != "p=inner\ndone\n" {
		t.Errorf("the action should see the frame's arguments: got %q, want p=inner", got)
	}
	// A `return` is the only control flow this judges. Anything else still
	// standing is a body that did not finish, so it has no status of its
	// own to announce — measured on zsh 5.9.2: `g() { exit 3; }; g` writes
	// no E, and a `break` taken out of a body writes the one E the failure
	// before it already fired rather than a second.
	if got, _ := run(t, `trap 'echo E' ERR; g() { exit 3; }; g; echo done`, withSem(s)); got != "" {
		t.Errorf("an exit out of the body: got %q, want no E", got)
	}
	// Whether a `break` leaves a call at all is a question of its own, and
	// has to be answered for there to be a break out of the body to judge.
	brk := s
	brk.FunctionCallIsALoopControlBoundary = No
	if got, _ := run(t, `trap 'echo E' ERR; g() { false; break; }; for i in 1; do g; done; echo done`, withSem(brk)); got != "E\ndone\n" {
		t.Errorf("a break out of the body: got %q, want the one E the failure fired", got)
	}
	// And a dialect that judges the *call* reads the caller's, which is
	// what makes the row above a measurement of where rather than of when.
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapRefiresWhereItWasSetFirst
	if got, _ := run(t, `trap 'echo v=$v' ERR; g() { local v=in; return 1; }; v=out; g; echo done`, withSem(s)); got != "v=out\ndone\n" {
		t.Errorf("a dialect that judges the call should see the caller's: got %q, want v=out", got)
	}
}

func TestTheRefiringAxisIsRefusedOnlyWhereItDecides(t *testing.T) {
	// An unanswered axis refuses by name rather than guessing — and this
	// one is reached only where an ERR trap exists to fire twice, which is
	// why dash, whose answer to `trap … ERR` is a refusal, leaves it
	// unanswered and never meets it.
	s := errSem()
	s.ErrTrapRunsInsideFunctions = Yes
	s.ErrTrapRefiresForTheCommandItFiredInside = ErrTrapRefiringUnspecified
	got, _ := run(t, `trap 'echo E' ERR; g() { false; }; g; echo done`, withSem(s))
	if !strings.Contains(got, "no dialect was chosen") {
		t.Errorf("got %q, want the axis refused", got)
	}
	if out, st := run(t, `g() { false; }; g; echo done`, withSem(s)); out != "done\n" || st != 0 {
		t.Errorf("a failing call with no ERR trap should need no answer: got %q status %d", out, st)
	}
	if out, st := run(t, `trap 'echo E' ERR; g() { true; }; g; echo done`, withSem(s)); out != "done\n" || st != 0 {
		t.Errorf("a call that succeeded should need no answer: got %q status %d", out, st)
	}
	// And a shell that keeps the trap out of the calls it was not set in
	// judges the body nowhere, so it is not put the question either — with
	// the refiring axis unanswered it still writes the one E for the call
	// and refuses nothing. The end of a call is reached whether or not the
	// trap has business there, so refusing at every one of them refused a
	// dialect for a question it was never asked: the ash column, which has
	// not answered whether the trap runs inside a function, printed that
	// refusal twice for `g(){ false; }; g` and once for a body that failed
	// by `return` alone — where nothing had been asked at all.
	s.ErrTrapRunsInsideFunctions = No
	for _, src := range []string{
		`trap 'echo E' ERR; g() { false; }; g; echo done`,
		`trap 'echo E' ERR; g() { return 1; }; g; echo done`,
		`trap 'echo E' ERR; g() { { false; }; }; g; echo done`,
	} {
		if out, st := run(t, src, withSem(s)); out != "E\ndone\n" || st != 0 {
			t.Errorf("the trap kept out of calls should refuse nothing: %s gave %q status %d", src, out, st)
		}
	}
	// And where the *frame* axis is the unanswered one, the end of a call
	// adds no refusal of its own. The refusal belongs to the failing
	// statement inside the body, which runErrTrap reaches and already
	// makes: one for `g(){ false; }`, one for a body whose group reports
	// the failure — the group ran a statement, so it is not judged again and
	// is not a second place to be asked (#3344) — and none at all for a body
	// that failed by `return` alone, since that statement is never judged. Counted rather than
	// matched, because the regression was a *second* copy of a complaint
	// that was already correct.
	s.ErrTrapRunsInsideFunctions = Unspecified
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`trap 'echo E' ERR; g() { false; }; g; echo done`, 1},
		{`trap 'echo E' ERR; g() { { false; }; }; g; echo done`, 1},
		{`trap 'echo E' ERR; g() { return 1; }; g; echo done`, 0},
	} {
		out, _ := run(t, tc.src, withSem(s))
		if n := strings.Count(out, "no dialect was chosen"); n != tc.want {
			t.Errorf("%s: refused %d times, want %d — %q", tc.src, n, tc.want, out)
		}
		if !strings.HasSuffix(out, "E\ndone\n") {
			t.Errorf("%s: got %q, want the one E for the call at the end", tc.src, out)
		}
	}
}

func TestErrexitIsUnchangedByTheOnceRule(t *testing.T) {
	// `set -e` shares checkErrExit with the trap, so the guard that stops
	// the second firing had to leave it alone: a failure inside a compound
	// ends the script exactly once, and the exemptions still exempt.
	s := errSem()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a group ends the script", `set -e; { true; false; }; echo done`, "", 1},
		{"a loop ends the script", `set -e; for i in 1; do false; done; echo done`, "", 1},
		{"an if body ends the script", `set -e; if true; then false; fi; echo done`, "", 1},
		{"the trap runs first, once", `trap 'echo E' ERR; set -e; { true; false; }; echo done`, "E\n", 1},
		{"a tested condition is exempt", `set -e; if false; then :; fi; echo ok`, "ok\n", 0},
		{"an || operand is exempt", `trap 'echo E' ERR; set -e; { false; } || echo or; echo done`, "or\ndone\n", 0},
		{"a tested compound fires nothing", `trap 'echo E' ERR; if { false; }; then echo t; else echo f; fi`, "f\n", 0},
		{"a non-final && operand is exempt", `trap 'echo E' ERR; set -e; { false; } && echo x; echo done`, "done\n", 0},
		{"a loop condition is exempt", `set -e; while { false; }; do :; done; echo done`, "done\n", 0},
	} {
		got, st := run(t, tc.src, withSem(s))
		if got != tc.want || st != tc.status {
			t.Errorf("%s: got %q status %d, want %q status %d", tc.name, got, st, tc.want, tc.status)
		}
	}
}
