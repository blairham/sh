// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// These tests name axes, never shells. Which dialect answers which way is
// dialect/<shell>'s to assert; what is pinned here is that a subshell's
// working traps reset unconditionally, that only the *listing* is the axes'
// to keep, and that the substrate refuses when no dialect answered a listing
// that needs one.

// trapSem answers just enough for a test about the subshell listing: how a
// listed action is quoted, and every keep-axis No until a test turns one on.
func trapSem() Semantics {
	s := CoreSemantics()
	s.TrapQuoting = ListingQuoteAlwaysEscaped
	s.TrapBodyRunsWhatParsed = Yes
	s.SignalHandlerSeesEarlierStatus = No
	s.TrapPrintsBareWithConditions = No
	s.SubshellKeepsTrapListing = No
	s.PipelineElementKeepsTrapListing = No
	s.BackgroundJobKeepsTrapListing = No
	s.KeptTrapListingIncludesExit = Yes
	s.SubshellHidesInheritedIgnoredTraps = No
	s.SubshellRunsOnAfterSignallingTheShell = Yes
	return s
}

func TestSubshellResetsAHandledTrapUnconditionally(t *testing.T) {
	// The reset is POSIX and unanimous, so it is not an axis: whichever way
	// the listing goes, the handled trap is gone from the working state.
	// The listing here is the reset made visible.
	s := trapSem()
	if got, _ := run(t, `trap 'echo x' USR1; (trap); echo done`, withSem(s)); got != "done\n" {
		t.Errorf("reset listing: got %q, want %q", got, "done\n")
	}
	// And gone in fact, not only from the listing: a subshell that aims the
	// signal at the shell reaches the *parent's* trap, after the subshell —
	// the handler must not run inside it.
	got, _ := run(t, `trap 'echo x' USR1; (kill -USR1 $$; echo sub); echo done`, withSem(s))
	if got != "sub\nx\ndone\n" {
		t.Errorf("parent handles after the subshell: got %q, want %q", got, "sub\nx\ndone\n")
	}
}

func TestSubshellKeepsAnIgnoredTrap(t *testing.T) {
	s := trapSem()
	// Listed where the dialect lists it…
	if got, _ := run(t, `trap '' USR2; (trap); echo done`, withSem(s)); got != "trap -- '' USR2\ndone\n" {
		t.Errorf("inherited ignore listed: got %q", got)
	}
	// …hidden where it hides it, while one the subshell sets itself shows.
	s.SubshellHidesInheritedIgnoredTraps = Yes
	if got, _ := run(t, `trap '' USR2; (trap); echo done`, withSem(s)); got != "done\n" {
		t.Errorf("inherited ignore hidden: got %q", got)
	}
	if got, _ := run(t, `(trap '' INT; trap); echo done`, withSem(s)); got != "trap -- '' INT\ndone\n" {
		t.Errorf("a local ignore is listed even where inherited ones hide: got %q", got)
	}
	// Hidden or not, it still ignores: the signal aimed at the shell while
	// the ignore is inherited kills nobody.
	if got, _ := run(t, `trap '' INT; (kill -INT $$; echo sub); echo done`, withSem(s)); got != "sub\ndone\n" {
		t.Errorf("inherited ignore still working: got %q", got)
	}
}

func TestSubshellKeepsTheListingWhereTheAxisSaysSo(t *testing.T) {
	s := trapSem()
	s.SubshellKeepsTrapListing = Yes
	const src = `trap 'echo x' USR1; trap '' QUIT; (trap); echo done`
	want := "trap -- '' QUIT\ntrap -- 'echo x' USR1\ndone\n"
	if got, _ := run(t, src, withSem(s)); got != want {
		t.Errorf("kept listing: got %q, want %q", got, want)
	}
	// A command substitution is the same boundary — the save=$(trap) idiom.
	if got, _ := run(t, `trap 'echo x' USR1; s=$(trap); echo "[$s]"`, withSem(s)); got != "[trap -- 'echo x' USR1]\n" {
		t.Errorf("kept in a command substitution: got %q", got)
	}
	// The kept listing answers a named condition too, not only the plain
	// form.
	s.TrapParsesOptions, s.TrapPrintsWithP = Yes, Yes
	if got, _ := run(t, `trap 'echo x' USR1; (trap -p USR1)`, withSem(s)); got != "trap -- 'echo x' USR1\n" {
		t.Errorf("kept listing through -p: got %q", got)
	}
}

func TestModifyingATrapDropsTheKeptListing(t *testing.T) {
	s := trapSem()
	s.SubshellKeepsTrapListing = Yes
	// Any set, ignore or reset shows the subshell's own state instead — and
	// an inherited ignore, being live rather than a picture, survives it.
	const src = `trap '' INT; trap 'echo x' USR1; (trap '' USR2; trap); echo done`
	want := "trap -- '' INT\ntrap -- '' USR2\ndone\n"
	if got, _ := run(t, src, withSem(s)); got != want {
		t.Errorf("after a set: got %q, want %q", got, want)
	}
	const reset = `trap 'echo x' USR1; (trap - USR1; trap); echo done`
	if got, _ := run(t, reset, withSem(s)); got != "done\n" {
		t.Errorf("after a reset: got %q, want %q", got, "done\n")
	}
}

func TestPipelineElementListingIsItsOwnBoundary(t *testing.T) {
	s := trapSem()
	s.PipelineElementKeepsTrapListing = Yes
	if got, _ := run(t, `trap 'echo x' USR1; trap | cat; echo done`, withSem(s)); got != "trap -- 'echo x' USR1\ndone\n" {
		t.Errorf("kept in a pipeline element: got %q", got)
	}
	s.PipelineElementKeepsTrapListing = No
	s.SubshellKeepsTrapListing = Yes
	if got, _ := run(t, `trap 'echo x' USR1; trap | cat; echo done`, withSem(s)); got != "done\n" {
		t.Errorf("a subshell answer does not leak into a pipeline: got %q", got)
	}
}

func TestBackgroundJobListingIsItsOwnBoundary(t *testing.T) {
	s := trapSem()
	s.BackgroundJobKeepsTrapListing = Yes
	if got, _ := run(t, `trap 'echo x' USR1; trap & wait; echo done`, withSem(s)); got != "trap -- 'echo x' USR1\ndone\n" {
		t.Errorf("kept in a background job: got %q", got)
	}
	s.BackgroundJobKeepsTrapListing = No
	if got, _ := run(t, `trap 'echo x' USR1; trap & wait; echo done`, withSem(s)); got != "done\n" {
		t.Errorf("reset in a background job: got %q", got)
	}
}

func TestKeptListingCrossesEveryBoundaryOrNone(t *testing.T) {
	// A `(trap)` inside a pipeline element crosses two boundaries, and the
	// listing survives only where both axes say keep — which is what one
	// shell's empty `(trap) | cat` against its own populated `(trap)` pins.
	s := trapSem()
	s.SubshellKeepsTrapListing = Yes
	s.PipelineElementKeepsTrapListing = No
	if got, _ := run(t, `trap 'echo x' USR1; (trap) | cat; echo done`, withSem(s)); got != "done\n" {
		t.Errorf("dropped at the pipeline boundary: got %q", got)
	}
	s.PipelineElementKeepsTrapListing = Yes
	if got, _ := run(t, `trap 'echo x' USR1; (trap) | cat; echo done`, withSem(s)); got != "trap -- 'echo x' USR1\ndone\n" {
		t.Errorf("kept across both: got %q", got)
	}
	s.SubshellKeepsTrapListing = No
	if got, _ := run(t, `trap 'echo x' USR1; (trap | cat); echo done`, withSem(s)); got != "done\n" {
		t.Errorf("dropped at the subshell boundary: got %q", got)
	}
}

func TestKeptListingIncludesExitIsItsOwnAxis(t *testing.T) {
	s := trapSem()
	s.SubshellKeepsTrapListing = Yes
	s.KeptTrapListingIncludesExit = Yes
	want := "trap -- 'echo E' EXIT\ntrap -- 'echo x' USR1\nmid\nE\n"
	if got, _ := run(t, `trap 'echo E' EXIT; trap 'echo x' USR1; (trap); echo mid`, withSem(s)); got != want {
		t.Errorf("EXIT included: got %q, want %q", got, want)
	}
	s.KeptTrapListingIncludesExit = No
	want = "trap -- 'echo x' USR1\nmid\nE\n"
	if got, _ := run(t, `trap 'echo E' EXIT; trap 'echo x' USR1; (trap); echo mid`, withSem(s)); got != want {
		t.Errorf("EXIT dropped from a kept listing: got %q, want %q", got, want)
	}
}

func TestSubshellNeverRunsAnInheritedExitTrap(t *testing.T) {
	// Unanimous, so no axis: the EXIT trap fires once, at the end of the
	// script, however the listing went.
	for _, keep := range []Answer{Yes, No} {
		s := trapSem()
		s.SubshellKeepsTrapListing = keep
		if got, _ := run(t, `trap 'echo E' EXIT; (echo sub); echo done`, withSem(s)); got != "sub\ndone\nE\n" {
			t.Errorf("keep=%v: got %q, want %q", keep, got, "sub\ndone\nE\n")
		}
	}
}

func TestTrapListingAxesAreAskedOnlyWhenTheyShow(t *testing.T) {
	// A subshell, a pipeline and a background job under a parent with traps
	// must not be refused for want of a dialect — the clone is not the
	// place the shells can be told apart.
	core := CoreSemantics()
	if got, st := run(t, `trap 'echo x' USR1; (echo hi); echo done`, withSem(core)); st != 0 || got != "hi\ndone\n" {
		t.Errorf("a plain subshell should need no answer: got %q status %d", got, st)
	}
	if got, st := run(t, `trap 'echo x' USR1; echo hi | cat`, withSem(core)); st != 0 || got != "hi\n" {
		t.Errorf("a plain pipeline should need no answer: got %q status %d", got, st)
	}
	// A listing with nothing inherited asks nothing either.
	if got, st := run(t, `(trap); echo done`, withSem(core)); st != 0 || got != "done\n" {
		t.Errorf("an empty listing should need no answer: got %q status %d", got, st)
	}
	// The listing of an inherited trap is where they disagree, so that is
	// what the core refuses.
	out, st := run(t, `trap 'echo x' USR1; (trap)`, withSem(core))
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("an inherited listing should be refused: got %q status %d", out, st)
	}
}

func TestSubshellSelfKillReachesTheParent(t *testing.T) {
	// A subshell is a pretend child process, so `kill -INT $$` inside one is
	// aimed at the shell at the top: the subshell carries on and the parent
	// dies when it next looks — with the trap the *parent* has, or without.
	s := trapSem()
	got, st := run(t, `(kill -INT $$; echo sub); echo done`, withSem(s))
	if got != "sub\n" || st != 130 {
		t.Errorf("untrapped: got %q status %d, want %q status 130", got, st, "sub\n")
	}
	// A trap the subshell sets for itself catches nothing aimed at a pid it
	// does not have.
	got, st = run(t, `(trap 'echo got' INT; kill -INT $$; echo sub); echo done`, withSem(s))
	if got != "sub\n" || st != 130 {
		t.Errorf("locally trapped: got %q status %d, want %q status 130", got, st, "sub\n")
	}
}

// And the dialect that runs its subshells in the shell's own process stops
// there instead, which is the whole of what the panel disagrees about.
//
// Measured 2026-09-05: `(kill -TERM $$; echo inner); echo outer` prints
// `inner` and dies in bash 5.3.15, bash 3.2.57, bash 3.2 run as `sh`, dash and
// zsh 5.9.2, and prints nothing and dies in ksh93u+. The shell ends and
// `outer` goes unprinted in all six, so what the axis moves is the one echo
// between them.
//
// Not a delivery race: the same answer on twenty-five runs of each under load,
// and a `sleep 0.3` between the kill and the echo does not change it. What
// separates ksh93 is that it gives the subshell no process of its own, so the
// signal lands on the very thing that was going to run the echo. Nothing here
// forks either, which is why the answer has to be chosen.
func TestASubshellMayStopWhereItSignalledTheShell(t *testing.T) {
	s := trapSem()
	s.SubshellRunsOnAfterSignallingTheShell = No
	got, st := run(t, `(kill -INT $$; echo sub); echo done`, withSem(s))
	if got != "" || st != 130 {
		t.Errorf("got %q status %d, want no output and status 130 — ksh93's shape", got, st)
	}
	// And the parent still stops: what the axis moves is the subshell's own
	// next command, never whether the shell ends.
	got, st = run(t, `(kill -INT $$; echo sub)
echo done`, withSem(s))
	if got != "" || st != 130 {
		t.Errorf("with a command after it: got %q status %d, want no output and status 130", got, st)
	}
}

// An unanswered axis is refused rather than guessed at, which is the rule for
// every axis read while the script is running: this one is reached only by a
// `kill` at the shell's own pid from inside a subshell, so nothing else pays
// for it.
func TestAnUnansweredSubshellSignalAxisIsRefused(t *testing.T) {
	s := trapSem()
	s.SubshellRunsOnAfterSignallingTheShell = Unspecified
	got, _ := run(t, `(kill -INT $$; echo sub); echo done`, withSem(s))
	if !strings.Contains(got, "no dialect was chosen") {
		t.Errorf("got %q, want the axis refused", got)
	}
	// And a subshell that signals nothing never reaches it.
	if out, st := run(t, `(echo hi); echo done`, withSem(s)); st != 0 || out != "hi\ndone\n" {
		t.Errorf("a plain subshell should need no answer: got %q status %d", out, st)
	}
}

// pseudoSubshellSem is trapSem plus the pseudo-conditions, inherited-trap
// answers all No, so what fires is what the tests turn on.
func pseudoSubshellSem() Semantics {
	s := trapSem()
	s.TrapHasErrCondition = Yes
	s.TrapHasDebugCondition = Yes
	s.TrapHasReturnCondition = Yes
	s.TrapBodyRunsWhatParsed = Yes
	s.ErrTrapRunsInsideFunctions = No
	s.DebugTrapRunsInsideCalls = No
	s.ErrTrapRunsInSubshells = No
	s.DebugTrapRunsInSubshells = No
	return s
}

func TestLocalPseudoTrapsFireInASubshell(t *testing.T) {
	// The subshell axes are about *inherited* traps: one the subshell sets
	// for itself fires and lists whatever the axes say about inheritance.
	s := pseudoSubshellSem()
	if got, _ := run(t, `(trap 'echo err' ERR; false); echo done`, withSem(s)); got != "err\ndone\n" {
		t.Errorf("a local ERR trap fires: got %q", got)
	}
	if got, _ := run(t, `(trap 'echo err' ERR; trap); echo done`, withSem(s)); got != "trap -- 'echo err' ERR\ndone\n" {
		t.Errorf("a local ERR trap lists: got %q", got)
	}
}

func TestInheritedPseudoTrapsFollowTheirAxes(t *testing.T) {
	s := pseudoSubshellSem()
	if got, _ := run(t, `trap 'echo err' ERR; (false; echo sub); echo done`, withSem(s)); got != "sub\ndone\n" {
		t.Errorf("inherited ERR suppressed: got %q", got)
	}
	if got, _ := run(t, `trap 'echo err' ERR; (trap); echo done`, withSem(s)); got != "done\n" {
		t.Errorf("a suppressed inherited ERR is not listed: got %q", got)
	}
	s.ErrTrapRunsInSubshells = Yes
	if got, _ := run(t, `trap 'echo err' ERR; (false; echo sub); echo done`, withSem(s)); got != "err\nsub\ndone\n" {
		t.Errorf("inherited ERR carried: got %q", got)
	}
	if got, _ := run(t, `trap 'echo err' ERR; (trap); echo done`, withSem(s)); got != "trap -- 'echo err' ERR\ndone\n" {
		t.Errorf("a carried inherited ERR is listed: got %q", got)
	}
}
