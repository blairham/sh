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

// And the same wait is not ended by a child's death **that does count** either,
// which is the other half of the rule and was the half still missing.
//
// Every job a `wait` is about raises that condition as it ends, so a wait that
// came back on it came back on its own first job — and the children still
// running were reaped after the script had finished, with nothing left to run
// their handler. The trap fired once where bash fires once per child, and the
// wait reported the signal rather than the job.
//
// Measured 2026-09-23 against bash 5.3.20, three background sleeps ending a
// tenth of a second apart and a counting trap:
//
//	n=0; trap 'n=$((n+1))' CHLD
//	sleep 0.1 & sleep 0.2 & sleep 0.3 &
//	wait; echo "status=$? n=$n"
//
// bash prints `status=0 n=3`. This shell printed `status=148 n=1`.
//
// Three children rather than one, because one is what the test above already
// holds and it passed against the defect: a single job's arrival lands as the
// wait is ending anyway, so nothing is left behind to be lost. The count is
// the discriminator.
//
// # The count is this shell's to promise, and for a while it was not keeping it
//
// All three cases here flaked on required checks — `status=0 n=2` on ubuntu,
// `n=0` and a lost `T` on macOS — on diffs with no Go in them at all (#4571).
//
// The reading that fits a real shell is that `SIGCHLD` does not queue and the
// kernel had coalesced two arrivals into one, which would make an exact count
// something no shell can promise. **That is not what this is.** The kernel's
// signal is dropped for this condition here and the arrival is raised by the
// interpreter itself — see Runner.childReaped, which says so and says why —
// so nothing outside this program gets a vote on the number.
//
// What the runs were catching was an ordering inside the shell: a job
// published itself as finished, which is what releases the `wait`, and only
// then recorded the child's death. The shell came back from the wait and ran
// to the end of the script while the goroutine that had released it was still
// on its way to the record. Widening that gap by two milliseconds reproduced
// both CI failures byte for byte; closing it — the record now happens inside
// the finish, before the close — makes all three counts here exact again. See
// Runner.jobReaped and TestAJobsEndingIsRecordedBeforeTheCloseThatReleasesAWait,
// which pins the ordering directly rather than by how often it holds.
//
// So the number stays. Loosening it to "at least one" would not even have
// stopped the flake: two of the three failures were an arrival that never
// arrived at all.
func TestAWaitIsNotEndedByTheDeathOfAChildItIsWaitingFor(t *testing.T) {
	const src = "n=0\ntrap 'n=$((n+1))' CHLD\n" +
		"/bin/sleep 0.1 &\n/bin/sleep 0.2 &\n/bin/sleep 0.3 &\n" +
		"wait\n" +
		"printf 'status=%s n=%s' \"$?\" \"$n\"\n"
	out, status := run(t, src, nil)
	if want := "status=0 n=3"; out != want || status != 0 {
		t.Errorf("got %q at status %d, want %q at 0", out, status, want)
	}
}
