// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

// A substitution's pipe outlives the command that named it while one of this
// shell's own descriptors is open on it.
//
// The arrangement this is about is an open that does not wait for a peer —
// `O_NONBLOCK` on the reading end — which is what a shell's `sysopen -o
// nonblock` does and what no redirection does. A blocking open parks in the
// kernel until the writer arrives, so the two rendezvous while the command
// that named the path is still running and the lifetime never comes up; a
// nonblocking one returns at once, the command ends, and the pipe used to be
// unlinked out from under a writer that had not yet noticed the reader.
//
// Written with two registered builtins rather than with a dialect's, because
// the rule is the core's: any embedder that opens a path a substitution
// produced and keeps the descriptor is in the same position. The first
// opens without waiting and hands the descriptor to the shell, exactly as
// Runner.SetDescriptor's callers do; the second reads it in a later command,
// which is the boundary the pipe has to survive.

// nonblockingOpener registers the two builtins and returns the setup.
func nonblockingOpener(t *testing.T) func(*Runner) {
	t.Helper()
	// The file the first builtin opened, so the second can read it. A shell
	// finds it in the descriptor table; a test builtin may keep it beside
	// one, and what the lifetime turns on is that the table holds it.
	var opened *os.File
	return func(r *Runner) {
		r.Register("nbopen", func(r *Runner, _ context.Context, args []string) int {
			// Long enough for the writer's poll to have backed off to its
			// longest interval, which is what makes this deterministic
			// rather than a race: from here the open and the end of this
			// command are microseconds apart, and the writer will not look
			// again for milliseconds. Without the lifetime rule its next
			// look is at a name that has been taken away, which it reads as
			// "nobody opened it" — so the substituted command never runs.
			time.Sleep(20 * time.Millisecond)
			// Retried on EINTR, which is the whole difference between the
			// raw syscall and os.OpenFile: Go preempts goroutines with a
			// signal, so any raw syscall in a Go program can come back
			// interrupted, and a loaded runner is when it does. The
			// interrupted open made `main` red on macOS and exercised
			// nothing this case is about (#1909). os.OpenFile is not the
			// fix here — the flag has to be on at open time and off
			// afterwards, which is the whole of what this helper is for.
			var fd int
			var err error
			for {
				fd, err = syscall.Open(args[0], syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
				if !errors.Is(err, syscall.EINTR) {
					break
				}
			}
			if err != nil {
				t.Errorf("nbopen %s: %v", args[0], err)
				return 1
			}
			// Blocking from here on, exactly as a shell's own nonblocking
			// open does with the flag once it has served its purpose.
			if err := syscall.SetNonblock(fd, false); err != nil {
				t.Errorf("clearing nonblock: %v", err)
				return 1
			}
			opened = os.NewFile(uintptr(fd), args[0])
			r.SetDescriptor(9, opened)
			return 0
		})
		r.Register("nbread", func(r *Runner, _ context.Context, _ []string) int {
			f := opened
			if f == nil {
				t.Error("nothing was opened")
				return 1
			}
			// Read until something arrives, for up to two seconds. The
			// retry is not slack in the assertion: this shell's writer
			// *polls* for its reader rather than being forked into place,
			// so a reader that reads the instant it has opened can see the
			// pipe with no writer in it — which reads as end of file. A
			// script sleeps before it reads for the same reason. What is
			// being asserted is that the substituted command's output
			// arrives at all, and without the lifetime rule it never does:
			// the writer gives up when the name goes away, so this loop
			// runs out and answers empty.
			buf := make([]byte, 16)
			deadline := time.Now().Add(2 * time.Second)
			for {
				n, _ := f.Read(buf)
				if n > 0 || time.Now().After(deadline) {
					_, _ = r.Stdout.Write([]byte("[" + string(buf[:n]) + "]"))
					return 0
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

// TestAPipeSurvivesForADescriptorThisShellKeeps is #1750: the command that
// named the path finished before the writer's poll had seen the reader, the
// pipe was unlinked, and the poll read that as "nobody opened it" — so the
// substituted command never ran and the descriptor answered end of file.
func TestAPipeSurvivesForADescriptorThisShellKeeps(t *testing.T) {
	// Ten runs, because the failure this fixes is a race the writer's first
	// poll wins most of the time: two runs in ten answered end of file
	// before the change, and one that passed nine times proves nothing.
	for range 10 {
		out, st := runGrammar(t, "nbopen <(printf hi)\nnbread",
			func(d *syntax.Dialect) { d.ProcessSubstitution = true },
			nonblockingOpener(t))
		if out != "[hi]" {
			t.Fatalf("out = %q status = %d, want the substituted command's output", out, st)
		}
	}
}

// TestAPipeNobodyOpenedStillGoesAtOnce is the other side: nothing about the
// ordinary lifetime moves. A command whose substitution nobody opened has its
// pipe taken away at the end of that command, which is what keeps a long
// session from filling its directory — and what ends the writer's wait.
func TestAPipeNobodyOpenedStillGoesAtOnce(t *testing.T) {
	out, _ := runGrammar(t, "printf '[%s]' <(printf hi)",
		func(d *syntax.Dialect) { d.ProcessSubstitution = true }, nil)
	path := strings.TrimSuffix(strings.TrimPrefix(out, "["), "]")
	if path == "" {
		t.Fatalf("out = %q, want the pipe's path", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want the pipe gone with the command that named it", path, err)
	}
}
