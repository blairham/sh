// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os/signal"
	"strings"
	"syscall"
	"testing"
)

// disarmPipe is the belt to the subshell's braces.
//
// The trap above is written inside `( )` on purpose. A trap set at the *top
// level* is the process's own disposition -- trapSignal calls signal.Ignore
// for it -- and one test binary runs every test in the package, so an ignore
// left behind is inherited by every child a later test starts. That is not
// hypothetical: with the trap at the top level, `TestAKilledCommandsStatus`
// spawns a shell that kills itself with PIPE, the shell inherited the ignore,
// survived, and reported 0 where the test wants the signal's status -- green
// in isolation and red in the package, on both runners.
//
// A subshell's trap table is its own and never touches os/signal, which is
// what makes it the right spelling here and disarms the write just the same.
// The reset stays because the cost is one line and the failure it prevents is
// a *different* test going wrong later, which is the hardest kind to read.
func disarmPipe(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { signal.Reset(syscall.SIGPIPE) })
}

// The mirror image of zsh: a write into a closed descriptor fails the command
// and a write into a pipe nobody is reading does not, both in silence. One
// axis cannot carry the pair, which is what BrokenPipeWriteErrorFailsTheCommand
// exists for. Measured 2026-09-12 against ksh93 AJM 93u+ (#770).

const brokenPipeWrite = `( trap "" PIPE
s=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
i=0
while [ $i -lt 12 ]; do s="$s$s"; i=$((i+1)); done
{ echo "$s"; echo "ws=$?" > ws.txt; } | true )
cat ws.txt`

func TestABrokenPipeWriteDoesNotFailTheCommand(t *testing.T) {
	disarmPipe(t)
	out, _ := answersRun(t, brokenPipeWrite)
	if !strings.Contains(out, "ws=0") {
		t.Errorf("output = %q, want the failed write to leave the command at 0", out)
	}
	if strings.Contains(out, "write error") || strings.Contains(out, "I/O error") {
		t.Errorf("output = %q, want silence", out)
	}
}

// And the descriptor half, which this shell answers the other way -- the row
// that stops the new axis from being read as a rename of the old one.
func TestAClosedDescriptorWriteStillFailsTheCommand(t *testing.T) {
	out, st := answersRun(t, `echo hi >&-`)
	if st != 1 {
		t.Errorf("status = %d, want 1 -- this shell fails on a closed descriptor", st)
	}
	if strings.Contains(out, "write error") {
		t.Errorf("output = %q, want silence", out)
	}
}
