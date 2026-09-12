// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a try-always block does at run time (#1216).
//
// The flag is named and the shell that sets it is not. None of this is an axis
// on Semantics and that is deliberate: an axis records a *disagreement* about
// identical syntax, and one panel column has the construct — the other five
// call the keyword a syntax error, so there is no second answer to switch
// between. Every row was measured against zsh 5.9.2 on 2026-09-07 and the
// end-to-end half lives in dialect/zsh.

// tryAlways enables the construct, and the empty-body rule beside it because
// two rows below write `{ }` on purpose.
func tryAlways(d *syntax.Dialect) {
	d.TryAlways = true
	d.CloseBraceAlwaysReserved = true
	d.EmptyCompoundBody = true
}

func runTry(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st := runGrammar(t, src, tryAlways, nil)
	return strings.TrimSpace(out), st
}

// The second half runs however the first half ended, and the status afterwards
// is the *first* half's.
//
// The status pair is written both ways round on purpose. `{ false; } always
// { true; }` alone cannot tell "the first half's status survives" from
// "whatever ran last wins", because a shell that got it wrong and a shell that
// got it right differ only on the other diagonal.
func TestACleanupHalfRunsAndDoesNotReplaceTheStatus(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`{ echo t; } always { echo a; }`, "t\na", 0},
		{`{ true; } always { echo a; }; echo "?=$?"`, "a\n?=0", 0},
		{`{ false; } always { echo a; }; echo "?=$?"`, "a\n?=1", 0},
		// The 2x2, both diagonals.
		{`{ false; } always { true; }; echo "?=$?"`, "?=1", 0},
		{`{ true; } always { false; }; echo "?=$?"`, "?=0", 0},
		// And `$?` inside the second half is the first half's, which is what
		// a cleanup block reading it into a local depends on.
		{`{ (exit 5); } always { echo "inner=$?"; }; echo "outer=$?"`, "inner=5\nouter=5", 0},
		// Either half may be empty.
		{`{ } always { echo a; }`, "a", 0},
		{`{ echo t; } always { }`, "t", 0},
		// It is a group and not a subshell, so both halves' assignments escape.
		{`{ v=1; } always { w=2; }; echo "v=$v w=$w"`, "v=1 w=2", 0},
		// Nesting, in each half in turn.
		{`{ { echo t; } always { echo a; }; } always { echo b; }`, "t\na\nb", 0},
		{`{ echo t; } always { { echo a; } always { echo b; }; }`, "t\na\nb", 0},
	} {
		out, st := runTry(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// A failed expansion in the try half stops what it was for, runs the cleanup
// half anyway, and does not poison the cleanup half's own expansions.
//
// The last clause is the one worth a test: a failure that gave up its line
// leaves a flag set, and a construct that read the flag without clearing it
// would abandon itself over the *previous* command's failure (#1220). The
// construct has no heading of its own — both halves are command lists — so
// this row is what says it needs no clearing rather than that it was
// forgotten.
func TestAFailedExpansionInTheTryHalfStillRunsTheCleanup(t *testing.T) {
	out, _ := runTry(t, `f(){ { for i in a $((1/0)) b; do echo $i; done; } always { echo "A$((1+1))"; }; }; f`)
	// The complaint's wording is the dialect's; what is asserted here is that
	// the loop stopped before its body and the cleanup half computed 1+1.
	if strings.Contains(out, "\na\n") || !strings.Contains(out, "A2") {
		t.Errorf("said %q, want no loop body and a cleanup half that could still expand", out)
	}
}

// A redirection on the construct reaches both halves. It is the construct's
// because there is nowhere else to write one: a redirection after the first
// half ends it and makes the keyword a syntax error.
func TestARedirectionOnATryAlwaysBlockReachesBothHalves(t *testing.T) {
	out, st := runTry(t, `{ echo t; } always { echo a; } > /dev/null; echo done`)
	if out != "done" || st != 0 {
		t.Errorf("said %q status %d, want only `done`: the redirection reached neither half", out, st)
	}
}

// A `return` out of the first half runs the second and then goes on returning,
// carrying its own value.
//
// The `NOT-REACHED` after the construct is the load-bearing half: without it,
// a shell that swallowed the return entirely would print the same two lines.
func TestAReturnOutOfTheFirstHalfRunsTheSecondAndStillReturns(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`f(){ { echo t; return 3; } always { echo A; }; echo NOT-REACHED; }; f; echo "?=$?"`, "t\nA\n?=3", 0},
		{`f(){ { echo t; return 0; } always { echo A; }; echo NOT-REACHED; }; f; echo "?=$?"`, "t\nA\n?=0", 0},
		// `$?` inside the second half is the value being returned.
		{`f(){ { return 3; } always { echo "inner=$?"; }; }; f`, "inner=3", 3},
		// The second half's *own* return leaves the function too, and its
		// value is discarded: the status is the first half's 5, not 4. Written
		// with a value, because a bare `return` cannot tell the two apart.
		{`f(){ { (exit 5); } always { echo A; return 4; }; echo NOT-REACHED; }; f; echo "?=$?"`, "A\n?=5", 0},
	} {
		out, st := runTry(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// A `return` with nothing to return from ends the script, and then there is no
// frame to unwind through, so the second half is skipped.
//
// It is the same rule the `exit` rows below turn on, asked of the other
// transfer — and it needs a *different* predicate, which is the point: a
// `return` is caught by anything there is to return from, a subshell's
// inherited frame included, where an `exit` is caught only by a frame of the
// shell it is in.
//
// The axis has to be answered for these rows to be reachable, because a
// `return` at the top level is one of the places the panel disagrees: three
// shells end the script with the status given and bash refuses and carries on.
// Answered the first way, which is the shell that has this construct.
func TestATopLevelReturnSkipsTheCleanupHalf(t *testing.T) {
	ends := testSemantics()
	ends.ReturnOutsideAFunctionIsRefused = No
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`{ echo t; return 3; } always { echo A; }; echo NOT-REACHED`, "t", 3},
		// Depth of compound command is not what decides it.
		{`for i in 1; do { echo t; return 3; } always { echo A; }; done; echo NOT-REACHED`, "t", 3},
		{`if true; then { echo t; return 3; } always { echo A; }; fi; echo NOT-REACHED`, "t", 3},
		// While a subshell inside a function still has a frame to return
		// from, which is where this parts company with `exit`.
		{`f(){ ( { echo t; return 3; } always { echo A; } ); echo "sub=$?"; }; f`, "t\nA\nsub=3", 0},
	} {
		out, st := runGrammar(t, tc.src, tryAlways, withSem(ends))
		if got := strings.TrimSpace(out); got != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, got, st, tc.want, tc.status)
		}
	}
}

// An `exit` runs the cleanup halves inside function bodies and skips the ones
// at the shell's own top level.
//
// The four rows are a 2x2 over "lexical or called `exit`" and "inside a
// function or not", and the off-diagonals are what say the variable is where
// the *cleanup half* sits: a function called from the first half does not make
// a top-level cleanup half run, and a lexical `exit` does not stop one inside a
// function body from running.
func TestAnExitRunsTheCleanupHalvesItUnwindsThrough(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`{ echo t; exit 7; } always { echo A; }; echo NOT-REACHED`, "t", 7},
		{`f(){ { echo t; exit 7; } always { echo A; }; }; f; echo NOT-REACHED`, "t\nA", 7},
		{`g(){ exit 7; }; { echo t; g; } always { echo A; }; echo NOT-REACHED`, "t", 7},
		{`g(){ exit 7; }; f(){ { echo t; g; } always { echo A; }; }; f`, "t\nA", 7},
		// Nested frames each run their own, and the one outside them does not.
		{
			`f(){ { echo t; exit 7; } always { echo AF; }; }; ` +
				`g(){ { f; } always { echo AG; }; }; g`,
			"t\nAF\nAG", 7,
		},
		{`f(){ { echo t; exit 7; } always { echo AF; }; }; { f; } always { echo AT; }`, "t\nAF", 7},
		// A compound command at the top level is still the top level.
		{`for i in 1; do { echo t; exit 7; } always { echo A; }; done`, "t", 7},
		{`if true; then { echo t; exit 7; } always { echo A; }; fi`, "t", 7},
		{`{ { echo t; exit 7; } always { echo AN; }; } always { echo AO; }`, "t", 7},
		// A subshell is a shell of its own, so its top level is a top level
		// even inside a function — while a function called *within* it runs.
		{`f(){ ( { echo t; exit 7; } always { echo A; } ); echo "sub=$?"; }; f`, "t\nsub=7", 0},
		{
			`g(){ { echo t; exit 7; } always { echo A; }; }; ` +
				`f(){ ( g ); echo "sub=$?"; }; f`,
			"t\nA\nsub=7", 0,
		},
		// An `exit` written in the second half wins outright, with its own
		// status: `return 3` in the first half does not hold it back.
		{`f(){ { echo t; return 3; } always { echo A; exit 9; }; }; f; echo NOT-REACHED`, "t\nA", 9},
		// And an `exit` in the first half is not held back by the second's
		// `return`, which is the same pair the other way round.
		{`f(){ { echo t; exit 7; } always { echo A; return 4; }; echo NOT-REACHED; }; f`, "t\nA", 7},
	} {
		out, st := runTry(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// `break` and `continue` reach the second half, and when both halves transfer,
// the more far-reaching one wins whichever half wrote it.
//
// The last two rows are the discriminating pair. A rule of "the first half's
// transfer wins unless the second half returns or exits" fits every other row
// here, and predicts one pass for `break` in the first half against `continue`
// in the second — which runs all three.
func TestBreakAndContinueReachTheCleanupHalf(t *testing.T) {
	for _, tc := range []struct {
		src, want string
	}{
		{
			`for i in 1 2 3; do { echo t$i; [ $i = 2 ] && break; } always { echo A$i; }; done; echo after`,
			"t1\nA1\nt2\nA2\nafter",
		},
		{
			`for i in 1 2; do { [ $i = 2 ] && continue; echo body$i; } always { echo A$i; }; done`,
			"body1\nA1\nA2",
		},
		// A transfer written in the second half takes effect when the first
		// half made none.
		{`for i in 1 2 3; do { echo t$i; } always { echo A$i; break; }; done; echo after`, "t1\nA1\nafter"},
		{
			`for i in 1 2; do { echo t$i; } always { echo A$i; continue; }; echo body$i; done`,
			"t1\nA1\nt2\nA2",
		},
		// return out-ranks break from either side.
		{
			`f(){ for i in 1 2 3; do { echo t$i; return 3; } always { echo A$i; break; }; done; ` +
				`echo NOT-REACHED; }; f; echo "?=$?"`,
			"t1\nA1\n?=3",
		},
		{
			`f(){ for i in 1 2 3; do { echo t$i; break; } always { echo A$i; return 4; }; done; ` +
				`echo NOT-REACHED; }; f; echo "?=$?"`,
			"t1\nA1\n?=0",
		},
		// And continue out-ranks break from either side, which is the pair no
		// other reading of the rule predicts.
		{
			`for i in 1 2 3; do { echo t$i; continue; } always { echo A$i; break; }; ` +
				`echo body$i; done; echo after`,
			"t1\nA1\nt2\nA2\nt3\nA3\nafter",
		},
		{
			`for i in 1 2 3; do { echo t$i; break; } always { echo A$i; continue; }; ` +
				`echo body$i; done; echo after`,
			"t1\nA1\nt2\nA2\nt3\nA3\nafter",
		},
		// The status after a transfer is still the first half's.
		{`for i in 1 2 3; do { false; } always { echo A$i; break; }; done; echo "?=$?"`, "A1\n?=1"},
	} {
		out, _ := runTry(t, tc.src)
		if out != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, out, tc.want)
		}
	}
}

// When **both** halves ask a loop for something, the second half's count wins
// and the continue-ness is sticky.
//
// Found by mutation: `controlWins` decided ties with `>`, and the survivor
// pointed straight at these rows. All six are needed, because no rule that
// takes one half's transfer *whole* fits them — the last two are what rule out
// both simpler readings, since taking either half whole makes them a `break 2`
// and stops both loops.
func TestBothHalvesAskingALoopForSomethingTakeTheSecondHalvesCount(t *testing.T) {
	const nested = `for i in 1 2; do for j in a b; do { echo t$i$j; %s; } always { echo A$i$j; %s; }; ` +
		`echo body; done; echo inner$i; done; echo after`
	for _, tc := range []struct{ try, always, want string }{
		// The second half's count decides, in both directions.
		{"break", "break 2", "t1a\nA1a\nafter"},
		{"break 2", "break", "t1a\nA1a\ninner1\nt2a\nA2a\ninner2\nafter"},
		{"continue", "continue 2", "t1a\nA1a\nt2a\nA2a\nafter"},
		{"continue 2", "continue", "t1a\nA1a\nt1b\nA1b\ninner1\nt2a\nA2a\nt2b\nA2b\ninner2\nafter"},
		// And the continue-ness is sticky, so a `continue` in *either* half
		// makes the result a continue at the second half's count. These two
		// are the discriminating rows.
		{"continue", "break 2", "t1a\nA1a\nt2a\nA2a\nafter"},
		{"break 2", "continue", "t1a\nA1a\nt1b\nA1b\ninner1\nt2a\nA2a\nt2b\nA2b\ninner2\nafter"},
	} {
		src := fmt.Sprintf(nested, tc.try, tc.always)
		out, _ := runTry(t, src)
		if out != tc.want {
			t.Errorf("{ %s } always { %s }: said %q, want %q", tc.try, tc.always, out, tc.want)
		}
	}
}

// An error the shell reported and gave up over — what zsh calls an error
// condition — runs the second half and is then re-raised behind it, and one
// raised *by* the second half is cleared instead.
//
// Which is the construct's whole purpose: a cleanup block that itself trips
// over something must not turn a reported failure into a lost one, and must
// not abandon the caller either. The dialect has to say the error is fatal for
// these rows to be reachable at all, which is why the semantics vector is set
// rather than left at the core's.
func TestAnErrorConditionRunsTheCleanupHalfAndSurvivesIt(t *testing.T) {
	fatal := testSemantics()
	fatal.ReadonlyReassignmentFatal = Yes
	// The status a reported-and-abandoned error leaves is an axis of its own,
	// and these rows assert one — so it is named rather than inherited.
	fatal.FatalErrorStatusIsOne = Yes
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// Raised in the first half: reported, the cleanup runs, and the
		// statement is still abandoned — `after-f` never prints.
		{`f(){ { readonly r=1; r=2; } always { echo A; }; echo after-f; }; f`, "A", 1},
		// Raised in the second half: reported, the second half stops there,
		// and the caller carries on.
		{`f(){ { echo t; } always { readonly r=1; r=2; echo NOT-REACHED; }; echo after-f; }; f`, "t\nafter-f", 0},
		// An `exit` in the second half still wins over a first-half error.
		{`f(){ { readonly r=1; r=2; } always { echo A; exit 9; }; echo after-f; }; f`, "A", 9},
		// A `return` in the second half does not: the error out-ranks it.
		{`f(){ { readonly r=1; r=2; } always { echo A; return 4; }; echo after-f; }; f; echo "?=$?"`, "A", 1},
	} {
		out, st := runGrammar(t, tc.src, tryAlways, withSem(fatal))
		if got := strings.TrimSpace(stripComplaint(out)); got != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, got, st, tc.want, tc.status)
		}
	}
}

// The same rule when the dialect calls such an error *survivable* rather than
// fatal, which the substrate carries as a third kind of unwinding.
//
// Whether a reported error ends the script or abandons only the statement is
// an axis of its own and is orthogonal to this construct, so the construct has
// to answer identically either way: the second half runs, its own error is
// cleared, and the first half's is re-raised behind it. The rows above are the
// fatal shape of exactly these four claims.
func TestASurvivableErrorRunsTheCleanupHalfTheSameWay(t *testing.T) {
	abandons := testSemantics()
	abandons.ReadonlyReassignmentFatal = No
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// Raised in the first half: the cleanup runs and the statement is
		// still given up, so `after-f` never prints — while the *next*
		// statement does, which is what makes this the survivable kind.
		{
			`f(){ { readonly r=1; r=2; } always { echo A; }; echo after-f; }; f` + "\n" + `echo next`,
			"A\nnext", 0,
		},
		// Raised in the second half: cleared, so the caller carries on.
		{
			`f(){ { echo t; } always { readonly r=1; r=2; echo NOT-REACHED; }; echo after-f; }; f`,
			"t\nafter-f", 0,
		},
		// It out-ranks a `return` written in the second half, the way the
		// fatal shape out-ranks one.
		{
			`f(){ { readonly r=1; r=2; } always { echo A; return 4; }; echo after-f; }; f` + "\n" + `echo next`,
			"A\nnext", 0,
		},
		// And an `exit` in the second half still wins over it.
		{`f(){ { readonly r=1; r=2; } always { echo A; exit 9; }; echo after-f; }; f`, "A", 9},
	} {
		out, st := runGrammar(t, tc.src, tryAlways, withSem(abandons))
		if got := strings.TrimSpace(stripComplaint(out)); got != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, got, st, tc.want, tc.status)
		}
	}
}

// A survivable error in the try half out-ranks a `return` written in the
// second half, and this is the row that says so on its own.
//
// **Nothing measures it.** The construct belongs to one shell and the
// survivable shape of a reported error belongs to the axis another shell
// answers, so no binary has the combination — the rule here is the one the
// fatal shape *is* measured to have, applied to the substrate's other shape
// of the same thing. It has a test of its own because otherwise the rank is
// silently droppable: with it gone, the `return` escapes and the caller gets
// its line back, which is a different program.
//
// The `; echo SAME-LINE` is what tells the two apart. A give-up takes the
// rest of the *line* with it; a `return` only leaves the function.
func TestASurvivableErrorOutranksACleanupHalvesReturn(t *testing.T) {
	abandons := testSemantics()
	abandons.ReadonlyReassignmentFatal = No
	src := `f(){ { readonly r=1; r=2; } always { return 4; }; }; f; echo SAME-LINE` + "\n" + `echo NEXT-LINE`
	out, _ := runGrammar(t, src, tryAlways, withSem(abandons))
	got := strings.TrimSpace(stripComplaint(out))
	if got != "NEXT-LINE" {
		t.Errorf("said %q, want only NEXT-LINE: the error's give-up has to survive the cleanup half's return", got)
	}
}

// stripComplaint drops the lines a diagnostic wrote, so a row asserting on
// *control flow* is not also asserting on wording. The wording is the
// dialect's and is pinned in dialect/zsh.
func stripComplaint(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "read-only") || strings.Contains(line, "readonly") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// `set -e` firing is not a script's own `exit`, and the difference shows only
// inside a function.
//
// Both end the shell, and #1216 carried both as one controlExit with
// abandonRequested — so the construct could not tell them apart and took the
// `exit` answer for both, running a cleanup half the shell with the construct
// skips. Measured against zsh 5.9.2 on 2026-09-07 and again 2026-09-12
// (#1238).
//
// The top-level rows are the reason a probe has to enter a function: there
// both answers agree, so a test written outside one grades nothing.
func TestErrexitFiringSkipsACleanupHalfWhereAnExitRunsIt(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// An `exit` unwinds the frames and runs the cleanup halves on the
		// way out. The control for every row below it.
		{`f(){ { echo t; exit 7; } always { echo A; }; }; f`, "t\nA", 7},
		// `set -e` ends things where it stands: no cleanup half runs, and
		// nothing after the call does either.
		{`set -e; f(){ { echo t; false; } always { echo A; }; echo after-f; }; f; echo tail`, "t", 1},
		// Not even the ones further out — the frames are not unwound, they
		// are abandoned.
		{`set -e; f(){ { echo t; false; } always { echo A; }; }; ` +
			`g(){ { f; } always { echo B; }; }; g`, "t", 1},
		// The failure may come from anywhere the statement's status does: a
		// called function, or a subshell.
		{`set -e; g(){ false; }; f(){ { echo t; g; } always { echo A; }; echo after-f; }; f`, "t", 1},
		{`set -e; f(){ { echo t; ( exit 3 ); } always { echo A; }; echo after-f; }; f`, "t", 3},
		// At the top level the two answers agree, which is why the rows
		// above are written inside a function.
		{`set -e; { echo t; false; } always { echo A; }`, "t", 1},
		// The controls that keep this from being "with `set -e` on, no
		// cleanup half ever runs": a statement that succeeded, one whose
		// failure was tested, and one that failed inside an `||`.
		{
			`set -e; f(){ { echo t; true; } always { echo A; }; echo after-f; }; f; echo tail`,
			"t\nA\nafter-f\ntail", 0,
		},
		{
			`set -e; f(){ { echo t; false; } always { echo A; }; echo after-f; }; f || echo caught`,
			"t\nA\nafter-f", 0,
		},
		{
			`set -e; f(){ { echo t; false || true; } always { echo A; }; echo after-f; }; f`,
			"t\nA\nafter-f", 0,
		},
		// And an `exit` written in the *cleanup* half of a run `set -e`
		// stopped is unreachable — the half never runs, so the status stays
		// the failing command's rather than becoming the one it names.
		{`set -e; f(){ { echo t; false; } always { exit 5; }; }; f`, "t", 1},
	} {
		out, st := runTry(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// The two parameters a try-always block reports through, and recovers
// through (#1234).
//
// The names are the caller's — Runner.SetAlwaysBlockStatus takes them — so
// these are spelled neutrally here and the shell's own spelling is asserted
// in dialect/zsh. Every row was measured against zsh 5.9.2 on 2026-09-12 with
// that shell's names; see alwaysstatus.go for the table.
func alwaysStatus(r *Runner) { r.SetAlwaysBlockStatus("BLOCK_ERROR", "BLOCK_INTERRUPT") }

func runTryStatus(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st := runGrammar(t, src, tryAlways, alwaysStatus)
	return strings.TrimSpace(out), st
}

// What the pair reads, which is a question about *error conditions* and not
// about the status.
func TestTheAlwaysBlockParametersReportAnErrorCondition(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// Outside a half they are an ordinary integer parameter at -1 — a
		// value rather than an absence, which is what the `:-` row says.
		{`echo "[$BLOCK_ERROR][$BLOCK_INTERRUPT]"`, "[-1][-1]", 0},
		{`echo "${BLOCK_ERROR:-UNSET}"`, "-1", 0},
		// A nonzero status is not an error condition, and neither is a
		// `return`. These two are the rows a reading built on `$?` gets
		// wrong, and it gets them wrong in the quiet direction.
		{`f(){ { false; } always { echo "E=$BLOCK_ERROR"; }; }; f`, "E=0", 1},
		{`f(){ { return 3; } always { echo "E=$BLOCK_ERROR"; }; }; f`, "E=0", 3},
		// An error the shell reported and gave up over is. `break` with no
		// loop around it is one too in the shell that has the construct, and
		// it is not a row here: the core leaves that axis unanswered and
		// refuses the line by name instead, so what it reports is a
		// different question. dialect/zsh has that row.
		{
			`f(){ { echo $((1/0)); } always { echo "E=$BLOCK_ERROR"; }; }; f`,
			"sh: division by zero\nE=1", 2,
		},
		// The interrupt is 0 inside a half however the try half ended: it
		// reports an interrupt, and an error is not one.
		{
			`f(){ { echo $((1/0)); } always { echo "I=$BLOCK_INTERRUPT"; }; }; f`,
			"sh: division by zero\nI=0", 2,
		},
		// The half's value is one it takes on for the length of the half.
		// Both rows are needed: the first says it goes back, the second says
		// it goes back to what a script put there rather than to -1.
		{`f(){ { true; } always { :; }; echo "post=$BLOCK_ERROR"; }; f`, "post=-1", 0},
		{`BLOCK_ERROR=5; f(){ { true; } always { echo "in=$BLOCK_ERROR"; }; }; f; ` +
			`echo "post=$BLOCK_ERROR"`, "in=0\npost=5", 0},
		// Nested halves each report their own try half, and an error raised
		// inside the inner one is the outer one's too.
		{
			`f(){ { { echo $((1/0)); } always { echo "in=$BLOCK_ERROR"; }; } ` +
				`always { echo "out=$BLOCK_ERROR"; }; }; f`,
			"sh: division by zero\nin=1\nout=1", 2,
		},
	} {
		out, st := runTryStatus(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// Writing the pair is how a script recovers from an error condition and how it
// raises one, which is the half the parameters exist for.
func TestWritingTheAlwaysBlockParametersDecidesTheOutcome(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// Cleared: the function carries on from the statement after the
		// construct, and the complaint the try half already printed stays
		// printed. `$?` there is still the try half's, which the second row
		// reads before anything else can overwrite it.
		{
			`f(){ { echo $((1/0)); } always { BLOCK_ERROR=0; }; echo after-f; }; f; echo "?=$?"`,
			"sh: division by zero\nafter-f\n?=0", 0,
		},
		{
			`f(){ { echo $((1/0)); } always { BLOCK_ERROR=0; }; echo "after=$?"; }; f`,
			"sh: division by zero\nafter=2", 0,
		},
		// Raised where there was nothing: the status is 1 rather than the
		// number written, and it is an error condition rather than a
		// failure — an `||` does not catch it.
		{`f(){ { true; } always { BLOCK_ERROR=1; }; echo after-f; }; f; echo "?=$?"`, "", 1},
		{`f(){ { true; } always { BLOCK_ERROR=7; }; echo after-f; }; f; echo "?=$?"`, "", 1},
		{`f(){ { true; } always { BLOCK_ERROR=1; }; echo after-f; }; f || echo caught`, "", 1},
		// The other name raises the same way, from either starting point.
		{`f(){ { true; } always { BLOCK_INTERRUPT=1; }; echo after-f; }; f`, "", 1},
		{
			`f(){ { echo $((1/0)); } always { BLOCK_INTERRUPT=1; }; echo after-f; }; f`,
			"sh: division by zero", 2,
		},
		// The controls. A half that writes nothing changes nothing, in both
		// directions — without these the rule could be "an always half always
		// clears" or "always raises" and match half the rows above.
		{
			`f(){ { echo $((1/0)); } always { echo A; }; echo after-f; }; f; echo "?=$?"`,
			"sh: division by zero\nA", 2,
		},
		{`f(){ { true; } always { echo A; }; echo after-f; }; f; echo "?=$?"`, "A\nafter-f\n?=0", 0},
		{`f(){ { false; } always { echo A; }; echo after-f; }; f; echo "?=$?"`, "A\nafter-f\n?=0", 0},
		// Writing 0 where there was no error condition is not a rewrite
		// either, so a `return` still returns.
		{`f(){ { return 3; } always { BLOCK_ERROR=0; }; echo after-f; }; f; echo "?=$?"`, "?=3", 0},
	} {
		out, st := runTryStatus(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s: said %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}
