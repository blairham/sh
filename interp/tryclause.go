// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// The try-always block, `{ … } always { … }`: the second half runs however the
// first half ended.
//
// Every rule below was measured against zsh 5.9.2 on 2026-09-07, which is the
// only panel shell that has the construct — the other five call the keyword a
// syntax error where it stands. So none of this became an axis on
// [Semantics]: an axis records a *disagreement* about identical syntax, and
// there is no second answer to disagree with. It is the construct's own
// definition, and it lives here for the same reason a loop's is in compound.go.
//
// Four things are being decided at the end of the try half and they are not
// the same question:
//
//   - **Does the second half run at all?** Nearly always, and that is the
//     point of the construct: an ordinary failure, a `return`, a `break` or a
//     `continue` all reach it. The exception is a transfer that ends the shell
//     rather than being caught above the construct — see
//     [Runner.transferEndsTheShell], which is two different questions wearing
//     one name.
//   - **What `$?` is inside it.** The try half's status, including the value a
//     propagating `return` carried. Real code depends on this: powerlevel10k's
//     `gitstatus.plugin.zsh` opens its always half with `local -i ret=$?`.
//   - **What `$?` is afterwards.** The try half's, again — the second half's
//     own status is discarded. `{ true; } always { false; }` leaves 0 and
//     `{ false; } always { true; }` leaves 1, which is the off-diagonal pair
//     that says so.
//   - **Which control transfer wins** when both halves make one. A severity
//     order, and it is measured in both directions rather than assumed — see
//     [controlWins].

// tryClause runs `{ … } always { … }`.
func (r *Runner) tryClause(ctx context.Context, c *syntax.TryClause) error {
	// The redirections are the construct's and reach both halves, which is
	// measured: `{ echo t; } always { echo a; } > /dev/null` prints nothing
	// at all, and a redirection written between the halves instead is a
	// syntax error there.
	return r.withRedirs(ctx, c.Redirs, func() error {
		if err := r.runList(ctx, c.Try); err != nil {
			return err
		}
		ctl, status, sig := r.ctl, r.status, r.diedOfSig
		ctlDepth, abandon, abandonLine := r.ctlDepth, r.abandon, r.abandonLine
		if r.transferEndsTheShell(ctl, abandon) {
			// A transfer with nothing above it to catch it ends the shell
			// where it stands, and the second half never runs.
			return nil
		}
		// The second half starts from a cleared control state, abandonKind
		// included: `exit` sets controlExit and leaves abandonKind where it
		// found it, so an always half that exits after a try half that
		// *errored* would otherwise look like an error condition and have
		// its status thrown away. Measured — `{ readonly q=1; q=2; } always
		// { exit 9; }` exits 9, not 1.
		r.ctl, r.ctlDepth, r.abandon, r.abandonLine = controlNone, 0, abandonRequested, 0
		if err := r.runList(ctx, c.Always); err != nil {
			return err
		}
		if r.ctl == controlExit && !r.errorCondition() {
			// A hard exit from the second half wins outright, and takes its
			// own status with it: `{ return 3; } always { exit 9; }` exits 9,
			// and so does an always half whose `${x?word}` fires.
			return nil
		}
		switch {
		case r.errorCondition():
			// The second half's own error condition is discarded rather than
			// abandoning the statement around the construct.
			r.ctl, r.ctlDepth, r.abandon, r.abandonLine = ctl, ctlDepth, abandon, abandonLine
		case loopTransfer(r.ctl) && loopTransfer(ctl):
			// Both halves asked a loop for something, and then the halves are
			// not ranked against each other at all — see [loopTransfer].
			if ctl == controlContinue {
				r.ctl = controlContinue
			}
		case !controlWins(r.ctl, ctl):
			// Anything the second half transferred that the try half
			// out-ranks is dropped.
			r.ctl, r.ctlDepth, r.abandon, r.abandonLine = ctl, ctlDepth, abandon, abandonLine
		}
		// Whichever transfer won, the status is the try half's — and what
		// produced it travels with it, the way a subshell's does: a try half
		// whose last command a signal killed is a signal death out here too.
		r.status, r.diedOfSig = status, sig
		return nil
	})
}

// loopTransfer reports whether a transfer is one a *loop* consumes, which is
// the pair the two halves do not rank against each other.
//
// Where both halves ask a loop for something, neither wins outright: the
// second half's **count** is what takes effect, and the continue-ness is
// *sticky* — once either half has asked to continue, the result is a continue
// at that count. Measured 2026-09-07 over two nested loops, and all four
// combinations were needed, because no rule that picks one half's transfer
// whole fits them:
//
//	{ break; }     always { break 2; }     both loops stop
//	{ break 2; }   always { break; }       only the inner one stops
//	{ continue; }  always { continue 2; }  the outer loop advances
//	{ continue 2; }always { continue; }    the inner loop advances
//	{ continue; }  always { break 2; }     the *outer* loop advances
//	{ break 2; }   always { continue; }    the *inner* loop advances
//
// The last two are the ones that rule out both simpler readings. Taking the
// second half's transfer whole makes row five a `break 2` and stops both
// loops; taking the first half's whole makes row six a `break 2` and does the
// same. What actually happens is the count from the second half and the
// continue-ness from either — which is the shape a count plus a flag has, and
// this reconstructs it from the two fields the substrate keeps instead.
//
// The bug this replaces was found by mutation: `controlWins` had `>` where
// ties needed deciding, and a survivor pointed at exactly these rows (#1216).
func loopTransfer(c control) bool {
	return c == controlBreak || c == controlContinue
}

// controlWins reports whether the second half's transfer out-ranks the try
// half's.
//
// The order is exit, then an error condition, then return, then continue, then
// break, then nothing — and it is measured in *both* directions for each
// neighboring pair, because a rule read off one diagonal cannot be told from
// "whichever half went first wins":
//
//	{ return 3; } always { break; }      the return, and the loop runs on
//	{ break; }    always { return 4; }   the return again, so break loses both ways
//	{ continue; } always { break; }      the continue: three iterations
//	{ break; }    always { continue; }   the continue again, three iterations
//	{ exit 7; }   always { return 4; }   the exit, status 7
//	{ return 3; } always { exit 9; }     the exit, status 9
//
// The middle pair is the one that discriminates. "The try half wins unless
// the second half returns or exits" fits every row above except
// `{ break; } always { continue; }`, which that reading says stops the loop
// after one pass and which runs all three. Those two rows are also
// [loopTransfer]'s now, and agree with it: with equal counts the sticky
// continue-ness gives the same answer the rank does.
//
// An error condition sits between exit and return because a readonly
// reassignment in the try half still abandons the statement after an always
// half that returned — `{ readonly q=1; q=2; } always { return 4; }` inside a
// function reports, skips the rest of the function, and leaves 1 — while
// `always { exit 9; }` in its place exits 9.
//
// **`>` and `>=` cannot be told apart here**, and that is worth writing down
// because a mutation run says so and the reason is not obvious. Of the
// thirty-six pairs, twenty reach this call — an exit and an error condition
// from the second half are each returned or restored above, and two loop
// transfers are decided by count and stickiness — and exactly two of those
// twenty are rank ties:
//
//   - nothing against nothing, where the only difference is whether abandonKind
//     and its line are put back, and neither is read while the control is
//     controlNone.
//   - a `return` against a `return`, where the status is the try half's either
//     way and a return carries no count.
//
// So a `>=` mutant survives as an *equivalent* one rather than as an uncovered
// case. `>` is kept because the severity order is what the doc above states.
func controlWins(second, try control) bool {
	return controlRank(second) > controlRank(try)
}

func controlRank(c control) int {
	switch c {
	case controlExit:
		return 5
	case controlAbandon:
		return 4
	case controlReturn:
		return 3
	case controlContinue:
		return 2
	case controlBreak:
		return 1
	}
	return 0
}

// transferEndsTheShell reports whether a transfer out of the try half has
// nothing above the construct to catch it, in which case the always half is
// skipped.
//
// Two transfers can end the shell and they ask *different* questions, which is
// measured rather than tidied:
//
//   - `exit` is caught by a function frame of **this** shell. See
//     [Runner.insideFunctionCall] for the subshell half of that.
//   - `return` is caught by anything there is to return from, a subshell's
//     inherited frame included. `f(){ ( { return 3; } always { echo A; } ); }; f`
//     prints A and leaves the subshell at 3, where the same shape with `exit`
//     prints nothing — so the two cannot share one predicate.
//
// `break` and `continue` are not here even with no loop to leave: measured,
// `{ break; } always { echo A; }` at the top level complains about the loop
// and still runs the always half.
//
// **One flavor of hard exit is not modeled**, and it is written down rather
// than guessed at: `set -e` firing inside the try half skips the always half
// in zsh *even inside a function*, where a script's own `exit` there runs it —
// measured 2026-09-07. The runner carries both as controlExit with
// abandonRequested and cannot tell them apart, and the distinction abandonKind
// draws is a different one, so separating them is a change to the core enum
// that #1216 does not need. Tracked separately; the visible effect is a
// cleanup block that runs where zsh's would not, which is louder than it is
// wrong.
func (r *Runner) transferEndsTheShell(ctl control, abandon abandonKind) bool {
	switch ctl {
	case controlExit:
		switch abandon {
		case abandonError:
			// An error the shell reported and gave up over is what zsh calls
			// an *error condition*, and the whole point of the construct is
			// to clean up after one: the always half runs and the error is
			// re-raised behind it. Measured — `f(){ { readonly q=1; q=2; }
			// always { echo A; }; echo after-f; }; f` prints the complaint
			// and A, skips after-f, and leaves 1.
			return false
		case abandonParamError:
			// `${x?word}` is documented as exiting the shell, which is the
			// whole reason abandonKind tells it apart from the rest, and it
			// skips the always half even inside a function. Measured:
			// `f(){ { : ${x?boom}; } always { echo A; }; }; f` prints the
			// complaint and nothing else.
			return true
		}
		return !r.insideFunctionCall()
	case controlReturn:
		return !r.hasSomethingToReturnFrom()
	}
	return false
}

// errorCondition reports whether what the runner is carrying is an error it
// reported and gave up over, rather than a request to stop.
//
// It is the same distinction abandonKind was made for, asked here because the
// always half's own error condition is *cleared* on the way out where its own
// `exit` is obeyed.
//
// Both of the substrate's shapes for it, and they are one question rather than
// two: whether such an error is fatal or abandons only the statement is an
// axis of its own — Semantics.ReadonlyReassignmentFatal and its neighbors —
// and it is orthogonal to this construct, so a dialect that answers it either
// way has to get the same rule. Naming only the fatal shape would have made
// the behavior depend on an axis nobody asked about here.
func (r *Runner) errorCondition() bool {
	return r.ctl == controlAbandon ||
		(r.ctl == controlExit && r.abandon == abandonError)
}

// hasSomethingToReturnFrom reports whether a `return` has a frame to leave: a
// function call, or a file being sourced.
//
// Shared with the `return` builtin rather than spelled twice, because the two
// have to agree — a `return` the builtin obeys is one the try-always block
// must treat as caught, and a `return` it refuses is one that ends the script.
func (r *Runner) hasSomethingToReturnFrom() bool {
	return r.inFunc != "" || r.sourceDepth != 0
}

// insideFunctionCall reports whether execution is inside a function call of
// *this* shell.
//
// It is asked for one measured reason: an `exit` runs the always halves it
// unwinds through and skips the ones at the shell's own top level. The variable
// is where the always half sits and not where the `exit` came from, which took
// a 2x2 to establish — a function called from the try half does not make a
// top-level always half run, and a lexical `exit` in the try half does not stop
// one inside a function body from running:
//
//	{ exit 7; } always { echo A; }                          nothing
//	f(){ { exit 7; } always { echo A; }; }; f               A
//	g(){ exit 7; }; { g; } always { echo A; }               nothing
//	g(){ exit 7; }; f(){ { g; } always { echo A; }; }; f    A
//
// Depth of compound command is not it either: a `for`, a `while`, a `case`, an
// `if`, a nested group, an `eval` and a sourced file at the top level all skip
// it, and an anonymous function — which is a function — does not. Nested
// frames each run their own: `f(){ { exit 7; } always { echo AF; }; };
// g(){ { f; } always { echo AG; }; }; g` prints AF and AG, and the same shape
// with `g` unwrapped prints AF alone.
//
// **A subshell is a shell of its own here**, which is why this asks about
// funcFloor rather than about depth alone. `f(){ ( { exit 7; } always { echo
// A; } ); }; f` prints nothing, and so do the command-substitution and
// background spellings of it — while an always half inside a *function* called
// within that subshell does run. A plain `r.depth > 0` would see the frames
// the clone inherited and answer yes to all four.
func (r *Runner) insideFunctionCall() bool { return r.depth > r.funcFloor }
