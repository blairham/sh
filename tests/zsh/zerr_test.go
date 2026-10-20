// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// ZERR, and the `TRAP…` function spelling of a handler, measured against zsh
// 5.9.2 on 2026-09-14. See interp/trapfunction.go for the mechanism and
// interp/pseudotrap.go for the condition the second name resolves to.
//
// Two spellings and one slot, so most of what is asserted here is the *slot*:
// that setting either spelling replaces the other, that removing either
// removes both, and that the condition fires in exactly the places ERR
// already fires in. What is new is the naming, not the firing — folding into
// the existing ERR machinery rather than writing a second one is what keeps
// the suppression rules from having to be measured twice (#2771).

// The first row of the issue: the name zsh's own documentation uses, which
// this shell refused outright.
func TestZERRNamesTheErrCondition(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `trap 'echo caught' ZERR; false; echo done`)
	if st != 0 || out != "caught\ndone\n" {
		t.Errorf("out %q status %d, want the handler to have run", out, st)
	}
}

// And the second, which is the one that matters: a declaration parses
// everywhere, so a shell without the convention takes this line, defines the
// function, never calls it, and says nothing at all.
func TestATrapFunctionIsTheHandler(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `TRAPZERR() { echo caught; }; false; echo done`)
	if st != 0 || out != "caught\ndone\n" {
		t.Errorf("out %q status %d, want the function to have been called as the handler", out, st)
	}
}

// One slot under two names, read from both sides. A shell holding ZERR and
// ERR as separate conditions would pass one of these and fail the other,
// which is why neither is written on its own.
func TestZERRAndERRAreOneCondition(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"set under ERR, reset under ZERR", `trap 'echo E' ERR; trap - ZERR; false; echo done`},
		{"set under ZERR, reset under ERR", `trap 'echo E' ZERR; trap - ERR; false; echo done`},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != "done\n" {
			t.Errorf("%s: out %q status %d, want the reset to have reached the handler", tc.name, out, st)
		}
	}
}

// The listing says which of the two names the script used. A canonicalizing
// shell prints the same word twice here, which is what this one did.
func TestTheListingEchoesTheNameTheConditionWasGiven(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `trap 'echo E' ZERR; trap; trap 'echo E' ERR; trap`)
	want := "trap -- 'echo E' ZERR\ntrap -- 'echo E' ERR\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// A function-spelled handler lists as the function, byte for byte with what a
// function listing writes: there is no action text to print.
func TestATrapFunctionListsAsTheFunction(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `TRAPZERR() { echo Z; }; trap`)
	want := "TRAPZERR () {\n\techo Z\n}\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
	same, st := runZsh(t, t.TempDir(), `TRAPZERR() { echo Z; }; functions TRAPZERR`)
	if st != 0 || same != want {
		t.Errorf("functions listing %q status %d, want the same bytes %q", same, st, want)
	}
}

// Three removals, three routes into the same slot. A shell can get any one of
// them right on its own, so each is a row: naming the condition to `trap`
// replaces the function, a reset takes it away, and taking the function away
// untraps the condition.
func TestEitherSpellingRemovesTheOther(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`TRAPZERR() { echo Z; }; trap 'echo T' ZERR; false; echo done; functions TRAPZERR 2>&1; echo "fn=$?"`)
	// The `functions` lookup fails, which is itself a failure the handler
	// that was just installed fires for — the second T is what says the new
	// handler is really the one in place.
	want := "T\ndone\nT\nfn=1\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
	for _, tc := range []struct{ name, src string }{
		{"a reset", `TRAPZERR() { echo Z; }; trap - ZERR; false; echo done`},
		{"a one-word reset", `TRAPZERR() { echo Z; }; trap ZERR; false; echo done`},
		{"unset -f", `TRAPZERR() { echo Z; }; unset -f TRAPZERR; false; echo done`},
		{"unfunction", `TRAPZERR() { echo Z; }; unfunction TRAPZERR; false; echo done`},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != "done\n" {
			t.Errorf("%s: out %q status %d, want nothing left trapped", tc.name, out, st)
		}
	}
}

// And the two spellings of the *function* are one slot too: a second
// `TRAP…` function for the same condition replaces the first rather than
// sitting beside it, so only the later name is still defined.
func TestASecondTrapFunctionForOneConditionReplacesTheFirst(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`TRAPERR() { echo E; }; TRAPZERR() { echo Z; }; false; echo ---; functions`)
	want := "Z\n---\nTRAPZERR () {\n\techo Z\n}\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// The two controls the convention needs, so that it is about the *name* and
// not about four letters: a lower-case spelling is an ordinary function, and
// a TRAP-prefixed name whose suffix is no condition is an ordinary function
// that is still callable by that name.
func TestATrapFunctionNameIsReadCaseSensitivelyAndOnlyForRealConditions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trapzerr() { echo z; }; TRAPFOO() { echo f; }; false; TRAPFOO; echo done`)
	if st != 0 || out != "f\ndone\n" {
		t.Errorf("out %q status %d, want neither name read as a condition", out, st)
	}
}

// The convention is not ZERR's. Every condition this shell has answers to it,
// and a signal's handler is the case that says the name is resolved against
// the signal table rather than against a list of three words.
func TestTheConventionReachesEveryCondition(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `TRAPEXIT() { echo E; }; echo body`)
	if st != 0 || out != "body\nE\n" {
		t.Errorf("EXIT: out %q status %d, want the handler at the end", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `TRAPUSR1() { echo U; }; kill -USR1 $$; echo body`)
	if st != 0 || out != "U\nbody\n" {
		t.Errorf("USR1: out %q status %d, want the handler on delivery", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `TRAPDEBUG() { echo D; }; true; echo body`)
	if st != 0 || !strings.HasPrefix(out, "D\n") {
		t.Errorf("DEBUG: out %q status %d, want the handler before each command", out, st)
	}
}

// The handler is called as the function it is, so the condition arrives in
// `$1` the way it does for a signal, and the body's `local` is local.
//
// The number is the platform's for a signal, and EXIT is zero everywhere —
// the pseudo-conditions are counted on past the last signal the host has, so
// asserting one of those would be asserting this machine's signal table.
func TestATrapFunctionIsCalledAsAFunction(t *testing.T) {
	n := strconv.Itoa(int(syscall.SIGUSR1))
	out, st := runZsh(t, t.TempDir(), `TRAPEXIT() { echo "E:$1"; }; echo body`)
	if st != 0 || out != "body\nE:0\n" {
		t.Errorf("out %q status %d, want the condition's number in $1", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `TRAPUSR1() { echo "U:$1"; }; kill -USR1 $$; echo body`)
	if st != 0 || out != "U:"+n+"\nbody\n" {
		t.Errorf("out %q status %d, want the signal's own number in $1", out, st)
	}
	out, st = runZsh(t, t.TempDir(),
		`v=outer; TRAPZERR() { local v=inner; echo "$v"; }; false; echo "$v"`)
	if st != 0 || out != "inner\nouter\n" {
		t.Errorf("out %q status %d, want the body's `local` to be local", out, st)
	}
}

// Where the condition does *not* fire, which is the half a handler running on
// every non-zero status would fail. The same gate `set -e` uses: a status
// being tested is not a failure.
func TestZERRIsSuppressedWhereAFailureIsTested(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`TRAPZERR() { echo Z; }; if false; then :; fi; while false; do :; done; `+
			`false && echo a; ! true; false | true; echo done`)
	if st != 0 || out != "done\n" {
		t.Errorf("out %q status %d, want nothing fired: every status here is tested", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `TRAPZERR() { echo Z; }; true | false; echo done`)
	if st != 0 || out != "Z\ndone\n" {
		t.Errorf("out %q status %d, want a pipeline that really failed to fire", out, st)
	}
}

// And it fires with `set -e` off as well as on, which is what makes it a
// handler rather than an exit hook — with the option on the handler runs
// first and the script then stops.
func TestZERRFiresWithAndWithoutErrExit(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt errexit; TRAPZERR() { echo Z; }; false; echo unreached`)
	if st != 1 || out != "Z\n" {
		t.Errorf("out %q status %d, want the handler and then the stop", out, st)
	}
}

// Whether a function-spelled handler still answers for the condition inside a
// subshell follows the condition and not the spelling: ZERR is carried across
// that boundary in this dialect, so the failure inside fires and the subshell
// command's own failure fires again outside.
func TestATrapFunctionFiresInsideASubshell(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `TRAPZERR() { echo Z; }; (false); echo done`)
	if st != 0 || out != "Z\nZ\ndone\n" {
		t.Errorf("out %q status %d, want both sides of the boundary to fire", out, st)
	}
}
