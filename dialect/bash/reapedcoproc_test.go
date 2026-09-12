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
func TestASubshellDeliversTheReapNotice(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; ( : ); echo "n=${#CP[@]}"`)
	if want := "n=0\n"; st != 0 || out != want {
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
