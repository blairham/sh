// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a `>(cmd)` writes reaches the shell's output, and the command that
// named it is what waits for it.
//
// The body of a writing substitution is the one that has nobody waiting for
// it by construction: `<(cmd)` writes into the pipe, so the command that
// named the path reads it to an end, and `=(cmd)` has run to completion
// before the path exists. A `>(cmd)` writes into the shell's *own* output
// instead, which nothing in the shell reads and nothing downstream ends.
//
// Where a real shell forks that costs nothing — the body is a process holding
// the shell's standard output, so the bytes cannot be lost however late they
// are. Here the body is a goroutine writing to the caller's io.Writer, and
// the caller stops reading when the shell is done: the same guarantee has to
// be a wait. See removeProcSubs for the measurement (bash 5.2, zsh 5.9 and
// ksh93 all answer `[PIPE]`, 1200 runs, idle and loaded) and for the one
// spelling that is excepted.

// A body slower than the command that named it still lands, and lands first.
//
// The sleep is what makes this a statement rather than a coin toss. Without
// the wait the shell is finished microseconds after `tee` is, and two hundred
// milliseconds later there is no longer anybody reading what the body writes:
// the assertion fails every time, which is the property a race can never have
// in either direction. #2183 is the same shape with the sleep taken out — 4
// failures in 60 runs on Linux and none on macOS, which is how it reached
// `main` and then the merge train.
//
// The ordering is asserted as well as the presence, because the presence
// alone would pass a shell that collected the output at its very end. What
// the panel disagrees about is exactly that: zsh waits at the command —
// `[PIPE]AFTER` — while bash and ksh93 do not wait at all and their surviving
// process delivers `AFTER[PIPE]`. This takes zsh's side because a goroutine
// cannot outlive the shell the way their process does; the row says which
// side was taken, so a later axis has to move it deliberately.
func TestAWritingSubstitutionsBodyIsWaitedForByTheCommandThatNamedIt(t *testing.T) {
	for _, tc := range []struct {
		name, src string
	}{
		{
			"in a pipeline",
			`printf "PIPE\n" | tee >(read -r v; sleep 0.2; printf "[%s]" "$v") >/dev/null; printf "AFTER"`,
		},
		{
			"as a redirection operand",
			`printf "PIPE\n" > >(read -r v; sleep 0.2; printf "[%s]" "$v"); printf "AFTER"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if want := "[PIPE]AFTER"; out != want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0 — the body's output did not land before the shell went on", out, st, want)
			}
		})
	}
}

// Except where the script is the one still holding the pipe open.
//
// `exec > >(cat)` hands the shell's own output to a body that reads until
// that end closes — and the end is now the shell's. A shell waiting there is
// waiting for a descriptor only it can close, which is a shell that has
// stopped: the wait above has to be off for this shape, and it is off for the
// same reason the pipe's own removal is, so the two share the clause rather
// than testing it twice.
//
// zsh does not wait here either, which is what says the exception is the
// construct's and not this implementation's: `zsh -c 'exec > >(cat); echo hi'`
// returns at once where every other `>(cmd)` spelling with a slow body takes
// the body's full time. Measured 2026-09-12 against zsh 5.9, bash 5.2 and
// ksh93 on Linux.
func TestAShellStillHoldingTheWritingEndDoesNotWaitForTheBody(t *testing.T) {
	f, err := syntax.Parse(`exec > >(cat); printf "hi"`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	sem := testSemantics()
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Env: testPATH()})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = r.Run(context.Background(), f)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		// Fatal rather than Error: the goroutine is still blocked on a pipe
		// nothing will close, and letting the package carry on would report
		// the hang against whichever later test the binary times out in.
		t.Fatal("the shell waited for a body it was itself still writing to")
	}
}
