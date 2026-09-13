// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// Half of a reaped coprocess goes here and half of it stays.
//
// Measured 2026-09-12 on ksh93 (AJM 93u+ 2012-08-01): the end this shell
// *writes* goes with the coprocess, so the letter that writes answers the same
// `no query process` it answers before any coprocess was started (#2411).
func TestWritingToAReapedCoprocessIsRefused(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`print hi |& wait; print -p x; print "w=$?"; print done`)
	if want := "ksh: print: no query process [Bad file descriptor]\nw=1\ndone\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// And the end it reads stays, so the line the coprocess left in the pipe is
// still there. This is what separates this shell's answer from bash's, which
// drops both ends at once.
func TestReadingAReapedCoprocessStillAnswers(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`print hi |& wait; read -p a; print "1=$? a=[$a]"`)
	if want := "1=0 a=[hi]\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// The read end goes when a read of it finds nothing left, and the whole
// coprocess goes with it — which is shared with the other shell that has the
// letters and is not this axis's to decide.
func TestAReadFindingEndOfFileForgetsTheCoprocess(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`print hi |& read -p a; read -p b; print "2=$?"; read -p c; print "3=$?"`)
	if want := "2=1\nksh: read: no query process\n3=1\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// A coprocess started with `|&` has no name to publish its ends under, so the
// notice a *redirection* delivers in the shell with an array must not reach it
// (#2582). Measured 2026-09-13 on ksh93u+ 2012-08-01: an `echo x >&2` between
// the coprocess ending and a `print -p` changes nothing, and the write still
// succeeds at 0.
//
// The write end here goes with the coprocess at the reaping — see
// Semantics.ReapedCoprocessEnds — so a notice delivered early would turn this
// into the no-coprocess refusal, which is the answer ksh93 keeps for a
// coprocess that was never started.
func TestAnOutputDuplicationDoesNotRetireAnUnnamedCoprocess(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `true |&
echo x >&2
print -p hi
print "st=$?"
print done`)
	if want := "x\nst=0\ndone\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}
