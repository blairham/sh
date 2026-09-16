// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A body that dies of a signal takes the body with it and leaves the shell.
//
// `<(cmd)`, `>(cmd)`, `=(cmd)` and `$(cmd)` are each a *process of its own* in
// every shell on the panel, so a fatal signal one of them takes ends that
// process and the shell that named it runs the next command. Measured
// 2026-09-15 against bash 5.3: a builtin inside the body writing into a pipe
// whose reader has gone leaves `read x < <(…)` at status 0, and the same write
// inside `x=$(…)` leaves `$?` at 141 with the line after it still running.
//
// Here a body is a goroutine rather than a fork, so there was nowhere else for
// the death to land: Runner.Finish raised the signal for the body exactly as
// it does for the shell, and the raise ends *this* process — which is the
// shell. Two files of bash's own suite ended on SIGPIPE where bash exits 0,
// which is what #3015 recorded; they arrived with #2995, where the FIFO that
// had kept a reader alive became an anonymous pipe that breaks.
//
// The broken pipe is made here rather than inside the script, and that is what
// makes this a case rather than a stress loop. A shell's own `<(…)` breaks its
// pipe only if the body is still writing when the last reader goes, which is
// the machine's call and was 4 runs in 5 when this was measured by hand. A
// pipe whose reading end is already closed breaks on the first write, every
// time, on every machine.
//
// DieBySignal is a recorder rather than a real raise, for the reason the field
// exists: a test process that honored `kill -PIPE $$` would end the test
// binary. What it records is exactly what the fix is about — whether the
// driver is asked to end the process at all.
func TestASubstitutionBodyKilledBySignalLeavesTheShellRunning(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		out    string
		status int
	}{
		{
			// The reading spelling, which is the one bash's suite tripped
			// over. Nothing of the body's death is the shell's: `$?` is the
			// command that named it — `read` at 1, because the body stopped
			// before it wrote a line — and `after` runs.
			//
			// The ordering is the substitution's own and needs nothing held
			// open: end-of-file reaches `read` when the shell's end of the
			// pipe is let go, which is what happens *after* the body's Run
			// returns, so the body has already been through Finish by the
			// time the next command is reached.
			name:   "a reading substitution",
			src:    "read x < <(echo body >&2); echo $?; echo after",
			out:    "1\nafter\n",
			status: 0,
		},
		{
			// And the writing one, whose body writes into the shell's own
			// standard output rather than into the pipe.
			name:   "a writing substitution",
			src:    "true >(echo body >&2); echo $?; echo after",
			out:    "0\nafter\n",
			status: 0,
		},
		{
			// A command substitution is the same boundary and reaches Finish
			// by the same road. Its status *is* the shell's, so this is the
			// case that says the death is still recorded rather than
			// swallowed: 141 is what bash reports here.
			name:   "a command substitution",
			src:    "x=$(echo body >&2); echo $?; echo after",
			out:    "141\nafter\n",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var died atomic.Int64
			var out strings.Builder

			// A pipe with nothing at the reading end, which is what every
			// write into it meets EPIPE on.
			rd, wr, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := rd.Close(); err != nil {
				t.Fatal(err)
			}
			defer wr.Close()

			r := newTestRunner(t, &Runner{
				Stdout: &out, Stderr: wr,
				DieBySignal: func(sig syscall.Signal) error {
					died.Store(int64(sig))
					return nil
				},
			})
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			st, err := r.Run(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if got := died.Load(); got != 0 {
				t.Errorf("the shell was asked to die by signal %d; a body's death is not the shell's", got)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if got := out.String(); got != c.out {
				t.Errorf("output = %q, want %q", got, c.out)
			}
		})
	}
}
