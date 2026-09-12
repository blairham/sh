// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The mirror image of zsh: a write into a closed descriptor fails the command
// and a write into a pipe nobody is reading does not, both in silence. One
// axis cannot carry the pair, which is what BrokenPipeWriteErrorFailsTheCommand
// exists for. Measured 2026-09-12 against ksh93 AJM 93u+ (#770).

const brokenPipeWrite = `trap "" PIPE
s=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
i=0
while [ $i -lt 12 ]; do s="$s$s"; i=$((i+1)); done
{ echo "$s"; echo "ws=$?" > ws.txt; } | true
cat ws.txt`

func TestABrokenPipeWriteDoesNotFailTheCommand(t *testing.T) {
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
