// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"path/filepath"
	"syscall"
	"testing"
)

// Where a write aimed at a coprocess that has ended leaves the shell (#2582).
//
// The construct these need is a wait long enough that nothing about a
// goroutine's scheduling can be in doubt, spent inside a *single* builtin so
// that no child is reaped along the way — a `read` on a pipe nobody writes.
// A loop of builtins would be the same shape and a worse measurement: #2506
// showed a coprocess row whose value was a five-hundred-iteration loop's race
// going the other way 36% of the time on a busy machine.
func fifoIn(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "p")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Fatalf("mkfifo %s: %v", p, err)
	}
	return p
}

// A write through the name the shell published is where it notices, so the
// script gets the ambiguous redirect bash gives and carries on.
//
// Before this, the coprocess's ends closed the instant its body returned and
// the write was a broken pipe every time: nothing printed and status 141.
func TestAWriteToACoprocessThatHasEndedNoticesIt(t *testing.T) {
	dir := t.TempDir()
	fifoIn(t, dir)
	out, st := runBash(t, dir, `exec 9<>p
coproc CP { exit 0; }
read -t 0.5 z <&9
echo x >&${CP[1]}
echo "after=$?"
echo done`)
	want := "bash: line 4: ${CP[1]}: ambiguous redirect\nafter=1\ndone\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// And not the operator: an ordinary `>&2` after a coprocess has ended leaves
// the array where it was, which is what bash answers thirty runs of thirty.
// A rule keyed on every output duplication would pass the test above and fail
// this one, and `>&2` is on more lines than anything else it could have been
// keyed on.
func TestAnOrdinaryOutputDuplicationDoesNotNoticeACoprocessHasEnded(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; echo x >&2; echo "n=${#CP[@]}"`)
	if want := "x\nn=2\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// The control that says the notice is not a way to survive SIGPIPE: a copy
// the script parked on a number of its own is the same open file, the
// reaping does not close it, and writing it still ends the shell. Measured
// the same way in bash, thirty runs of thirty.
//
// The `n=0` in front of it is what separates this from a shell that simply
// had not noticed. It had; the duplicate dies anyway.
func TestADuplicateOfACoprocessWriteEndStillEndsTheShell(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`coproc CP { echo hi; }; exec 3>&${CP[1]}; wait; echo "n=${#CP[@]}"; echo x >&3; echo "w=$?"; echo done`)
	if want := "n=0\n"; st != 141 || out != want {
		t.Errorf("out %q status %d, want %q at 141", out, st, want)
	}
}

// And the control with no coprocess anywhere in it: a descriptor whose reader
// has gone is a broken pipe, and a broken pipe still ends the shell. 141 in
// bash 5.3, bash as `sh`, bash 3.2 and zsh alike, and the answer a fix that
// merely swallowed the signal would have changed.
func TestAGenuineBrokenPipeStillEndsTheShell(t *testing.T) {
	dir := t.TempDir()
	fifoIn(t, dir)
	out, st := runBash(t, dir, `exec 9<>p
exec 3> >(exit 0)
read -t 0.5 z <&9
echo foo >&3
echo "st=$?"
echo alive`)
	if want := ""; st != 141 || out != want {
		t.Errorf("out %q status %d, want %q at 141", out, st, want)
	}
}
