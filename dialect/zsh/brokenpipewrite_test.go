// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell reverses the panel's usual pairing: a write into a closed
// descriptor is a shrug and a write into a pipe nobody is reading is a
// failure. ksh93 is the mirror image, which is why the two errnos cannot
// share one axis. Measured 2026-09-12 against zsh 5.9.2 (#770).

// brokenPipeWrite builds 256 KiB -- four times any pipe buffer -- and writes
// it into a reader that has already gone, with SIGPIPE disarmed so the errno
// comes back to a writer that is still running rather than killing it. The
// failed write's own status is carried out through a file, because it belongs
// to a command inside the pipeline.
const brokenPipeWrite = `trap "" PIPE
s=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
i=0
while [ $i -lt 12 ]; do s="$s$s"; i=$((i+1)); done
{ echo "$s"; echo "ws=$?" > ws.txt; } | true
cat ws.txt`

func TestABrokenPipeWriteFailsTheCommandAndIsSaidTwice(t *testing.T) {
	out, _ := answersRun(t, brokenPipeWrite)
	if !strings.Contains(out, "ws=1") {
		t.Errorf("output = %q, want the failed write to report 1", out)
	}
	// Twice, and that is measured rather than a duplicate: the builtin's own
	// complaint and then the stream's.
	if n := strings.Count(out, "write error: broken pipe"); n != 2 {
		t.Errorf("said it %d times in %q, want twice", n, out)
	}
	// The builtin's own comes first, and it is the one carrying the name.
	// Compared line by line: both lines end in the same words, so a test
	// that asked where "echo:" sits relative to the last "write error"
	// passes whichever order they are in -- it did, and a reordering mutant
	// walked straight through it.
	var said []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "write error: broken pipe") {
			said = append(said, line)
		}
	}
	if len(said) != 2 {
		t.Fatalf("output = %q, want exactly two complaints", out)
	}
	if !strings.Contains(said[0], "echo:") {
		t.Errorf("first line is %q, want the builtin's own, which names it", said[0])
	}
	if strings.Contains(said[1], "echo:") {
		t.Errorf("second line is %q, want the stream's, which names nothing", said[1])
	}
}

// The other errno keeps this shell's own answer, which is the half a single
// axis would have destroyed: silent, and the command still succeeded.
func TestAClosedDescriptorWriteStillDoesNotFailTheCommand(t *testing.T) {
	out, st := answersRun(t, `echo hi >&-`)
	if st != 0 {
		t.Errorf("status = %d, want 0 -- this shell does not fail on a closed descriptor", st)
	}
	if strings.Contains(out, "write error") {
		t.Errorf("output = %q, want silence for a stream this command closed itself", out)
	}
}
