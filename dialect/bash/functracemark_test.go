// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What the `t` mark on a *function* actually does — `declare -ft f` carrying
// the DEBUG and RETURN traps into that function's body, which is the half that
// was recorded and never read.
//
// The letter has been accepted, stored and written back by `declare -Fp` since
// #3192, and nothing consulted it: a marked function behaved exactly like an
// unmarked one. That is the same shape the readonly half was filed as — a
// letter that is listed and inert — and it was the last two lines of
// `suite: trap.tests` (#4157).
//
// Measured 2026-09-24 on bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin`, with a DEBUG or RETURN trap set at the **top
// level** so that the only thing that can carry it inside a call is the mark.

// The three commands of `traced` each fire, and `untraced` fires none — so the
// mark is doing the carrying and not `set -T`, which is nowhere in the script.
func TestTheFunctionTraceMarkCarriesTheDebugTrap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const src = "inner() { :; }\n" +
		"traced() { :; inner; }\n" +
		"untraced() { :; inner; }\n" +
		"declare -ft traced\n" +
		"trap 'echo \"D ${FUNCNAME:-top}\"' DEBUG\n" +
		"traced\n" +
		"untraced\n" +
		"trap - DEBUG\n"
	// Six firings: the two calls and the `trap - DEBUG` at the top level,
	// and `traced`'s own three commands. **`inner` is in none of them**,
	// which is what says the mark stops at the body it is on rather than
	// traveling with the call — measured, and the reason the RETURN half
	// below reads the function that is ending rather than the one running.
	want := "D top\nD traced\nD traced\nD traced\nD top\nD top\n"
	if out, st := runBash(t, dir, src); out != want || st != 0 {
		t.Errorf("got %q (status %d)\nwant %q", out, st, want)
	}
}

// And the RETURN trap, which fires for the marked function alone: not for the
// unmarked one beside it and not for the unmarked one it calls.
func TestTheFunctionTraceMarkCarriesTheReturnTrap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const src = "inner() { :; }\n" +
		"traced() { :; inner; }\n" +
		"untraced() { :; inner; }\n" +
		"declare -ft traced\n" +
		"trap 'echo \"R ${FUNCNAME:-top}\"' RETURN\n" +
		"traced\n" +
		"untraced\n" +
		"trap - RETURN\n"
	if out, st := runBash(t, dir, src); out != "R traced\n" || st != 0 {
		t.Errorf("got %q (status %d)\nwant %q", out, st, "R traced\n")
	}
}

// And the sign does **not** take it back off, which is measured rather than
// assumed and is the shape a tidier reading would have got wrong: `declare +ft
// f` leaves the function tracing in bash 5.3.20, so the letter under a plus is
// not the mark's removal.
//
// The control for "this shell has not simply started tracing every function"
// is the `untraced` row above, which fires nothing.
func TestThePlusSignDoesNotTakeTheFunctionTraceMarkOff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const src = "f() { :; }\n" +
		"declare -ft f\n" +
		"declare +ft f\n" +
		"trap 'echo \"D ${FUNCNAME:-top}\"' DEBUG\n" +
		"f\n" +
		"trap - DEBUG\n"
	want := "D top\nD f\nD f\nD top\n"
	if out, st := runBash(t, dir, src); out != want || st != 0 {
		t.Errorf("got %q (status %d)\nwant %q", out, st, want)
	}
}

// A subshell is not a function and carries no mark, which is measured rather
// than assumed: the gate for a subshell is left asking `set -T` alone.
func TestTheFunctionTraceMarkDoesNotReachASubshell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const src = "f() { :; }\n" +
		"declare -ft f\n" +
		"trap 'echo D' DEBUG\n" +
		"( : )\n" +
		"trap - DEBUG\n"
	// One firing, for the `( : )` statement itself, and nothing from inside
	// it.
	if out, st := runBash(t, dir, src); out != "D\n" || st != 0 {
		t.Errorf("got %q (status %d)\nwant %q", out, st, "D\n")
	}
}
