// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// What this shell leaves behind when a coprocess ends: nothing at all.
//
// Measured 2026-09-12 on bash 5.3.15 — `declare -p CP` and `declare -p CP_PID`
// both answer `not found` once the shell has noticed, so the array is unset
// rather than emptied and `${CP[1]}` names no descriptor (#2411).
func TestAReapedCoprocessLeavesNoArray(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; wait; echo "n=${#CP[@]} set=[${CP+yes}${CP_PID+yes}]"`)
	if want := "n=0 set=[]\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// And the write that used to end the shell instead reports an ambiguous
// redirect and carries on, which is the whole of #2411: the target word
// expands to nothing, so nothing is written into a pipe with no reader.
func TestWritingToAReapedCoprocessIsAnAmbiguousRedirect(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; wait; echo two >&${CP[1]}; echo "w=$?"; echo done`)
	if want := "bash: line 1: ${CP[1]}: ambiguous redirect\nw=1\ndone\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// The notice arrives where the shell reaps a child and nowhere else. A
// coprocess that writes one line and exits has certainly ended by the time the
// builtins after it run, and the array is still there — so a shell retiring it
// at the top of every statement would fail this and pass everything above.
func TestBuiltinsDoNotDeliverTheReapNotice(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; :; :; :; echo "n=${#CP[@]}"`)
	if want := "n=2\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// The idiom that must keep working: reading a coprocess that has just ended.
func TestReadingACoprocessThatHasJustEnded(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; read -r a <&${CP[0]}; echo "a=$a n=${#CP[@]}"`)
	if want := "a=hi n=2\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// And a subshell delivers the notice with no `wait` written anywhere, which is
// what makes it asynchronous rather than something `wait` performs.
//
// # Why the two reads are here
//
// A notice about a coprocess that has ended needs a coprocess that has ended,
// and `coproc CP { echo hi; }; ( : )` does not say that anywhere: it assumes
// the body got there first. Nothing in either shell orders the two. Here the
// body is a goroutine and the subshell costs microseconds, so on a loaded
// runner the subshell arrived with nothing to reap and the array was still
// there — `n=2` where the row wanted `n=0`, three times in one day on three
// unrelated diffs, green on every rerun (#2661). Real bash loses the same race
// for the same reason, measured: `coproc CP { echo hi; sleep 0.2; }; ( : )`
// answers 2 in bash 5.3.15 and answers 2 here.
//
// So the ordering is written down instead of hoped for. Reading the near end
// to end-of-file is the one fact about the coprocess a script can establish
// without `wait`, and startCoproc finishes the job before it closes that end,
// which makes the end-of-file mean the job has ended rather than merely that
// it is nearly done. The `n=2` in the middle is the other half: it says the
// reads did not deliver the notice, so the subshell after them is still the
// only thing that could have — and TestEndOfFileDoesNotTakeBackTheArray is the
// same claim pinned on its own.
//
// Measured on bash 5.3.15, 2026-09-14: `b=1 n=2` then `after=0`, five runs.
//
// And the mutation that tells the two shapes apart, since the flake itself
// could not be made to happen here — 200 rounds of the old shape on Linux
// under ten spinning cores, 150 of them race-instrumented, and 30 runs of the
// whole package, all green, which is why the three sightings were all on the
// runner. Give the body a `sleep 0.2` and the question stops being a race at
// all: the old script answers `n=2` — the assertion it made was `n=0` — in
// this shell **and in bash 5.3.15**, while this one answers `after=0` in
// both. So the old row passed only for as long as the body won, and the
// runner is where it stopped winning.
func TestASubshellDeliversTheReapNotice(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }
read -r a <&${CP[0]}
read -r b <&${CP[0]}
echo "b=$? n=${#CP[@]}"
( : )
echo "after=${#CP[@]}"`)
	if want := "b=1 n=2\nafter=0\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// Reading the array dry is not the reaping: two reads that both find
// end-of-file leave it where it was. That is the half the two shells with the
// coprocess letters answer the other way, and it is why their rule lives in
// the read path rather than in this axis.
func TestEndOfFileDoesNotTakeBackTheArray(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; read -r a <&${CP[0]}; read -r b <&${CP[0]}; echo "b=$? n=${#CP[@]}"`)
	if want := "b=1 n=2\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}
