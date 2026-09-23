// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"
)

// A `wait` is not ended by an arrival that does not count, and the cost of
// getting this wrong is the rest of the job's body.
//
// The kernel raises SIGCHLD for every process this one reaps, and a background
// job's own children are among them — they are the *job's* children, not the
// shell's, and a real shell never hears about them. This wait took one for a
// trapped arrival, came back to run the handler, and the script carried on past
// it: the job was still on its first command, the shell reached the end of the
// script, and everything the job had left to do was lost.
//
// Measured 2026-09-23 against bash 5.3.20 under `env -i PATH=/usr/bin:/bin`:
//
//	trap 'echo T' CHLD; ( sleep 0.05; echo b ) & wait; echo END
//
// prints `b`, then `T`, then `END` — the job runs to its end, the trap fires
// once for the job itself, and the wait comes back after both. This shell
// printed `T` and `END` and never printed `b`.
//
// The order is the assertion and not just the set: `b` before `T` is what says
// the wait did not come back early, and a test that only counted the lines
// would pass against a shell that ran the handler first and lost nothing.
//
// **What this sees is the extra arrival, where the binary loses the line.**
// Put the defect back and the shipped shell prints `T END` with no `b` at all,
// while here the same script prints `b T T END`: the harness reaches the end of
// the script a shade later than a process does, so the job gets to finish
// either way. The extra `T` is the same root — one arrival too many — and it is
// what a test in this package can hold on to.
func TestAWaitIsNotEndedByAnArrivalThatDoesNotCount(t *testing.T) {
	const src = "trap 'echo T' CHLD\n" +
		"( /bin/sleep 0.05; echo b ) &\n" +
		"wait\n" +
		"echo END\n"
	out, status := run(t, src, nil)
	if want := "b\nT\nEND\n"; out != want || status != 0 {
		t.Errorf("got %q at status %d, want %q at 0", out, status, want)
	}
}

// And the job is one arrival however many children it reaps of its own, which
// is the same fact from the other side: two arrivals reached the shell here —
// the inner child's and the job's — and only the second was the shell's to
// hear.
func TestABackgroundJobIsOneChildTrapArrival(t *testing.T) {
	const src = "n=0\ntrap 'n=$((n+1))' CHLD\n" +
		"( /bin/sleep 0.05; : ) &\n" +
		"wait\n" +
		"printf 'n=%s' \"$n\"\n"
	out, status := run(t, src, nil)
	if want := "n=1"; out != want || status != 0 {
		t.Errorf("got %q at status %d, want %q at 0", out, status, want)
	}
}
