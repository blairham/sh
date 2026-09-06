// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A command run after a background job still finds a terminal on its output.
//
// The guard interp puts over a caller's writer costs a descriptor: os/exec
// connects a child straight to an *os.File and builds a pipe for anything
// else, so a stream that has been wrapped means every later command is handed
// a pipe — and a child asking whether its output is a terminal gets a
// different answer for the rest of the session. `&` wraps both of the shell's
// streams and leaves them wrapped, because the job outlives the statement, so
// this was the price of that.
//
// It is not a price a shell has to pay, and #735 is where that became clear:
// the guard exists because two goroutines writing one io.Writer is a data
// race, and two writers of one *os.File are not — the descriptor's own lock
// serializes them, and the kernel serializes two processes. So lockWriter
// leaves a file alone, and a shell whose streams are files hands its children
// the descriptors as it always did.
//
// The panel is unanimous, measured through a pseudo-terminal with `sleep &`
// between the two children: bash 5.3, bash 3.2, dash, ksh93 and zsh all
// answer that the second child's output is a terminal. So there is no axis
// here to ask a dialect about — only a shell that used to disagree with all
// five.
func TestAChildAfterABackgroundJobStillSeesATerminal(t *testing.T) {
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform, so there is no terminal to see")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = control.Close() }()

	// Drained on a goroutine of its own: a terminal has a buffer, and a shell
	// writing into one nobody is reading would stop rather than finish.
	var mu sync.Mutex
	var drawn strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		for {
			n, err := control.Read(buf)
			if n > 0 {
				mu.Lock()
				drawn.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	const src = `/bin/sh -c 'test -t 1 && echo one-tty || echo one-pipe'
sleep 0.05 &
/bin/sh -c 'test -t 1 && echo two-tty || echo two-pipe'
wait
`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := testSemantics()
	dg := PosixDiagnostics()
	r := newTestRunner(t, &Runner{
		Stdout: terminal, Stderr: terminal, Semantics: &sem, Diagnostics: &dg, Env: testPATH(),
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	settle(r)
	// The shell's end goes first, so the reader above sees an end of input
	// rather than waiting for one.
	_ = terminal.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the terminal never ended")
	}

	mu.Lock()
	got := drawn.String()
	mu.Unlock()
	if !strings.Contains(got, "one-tty") {
		t.Fatalf("the first child did not see a terminal at all, so this proves"+
			" nothing about the second: %q", got)
	}
	if !strings.Contains(got, "two-tty") {
		t.Errorf("a child run after a background job was handed a pipe where every"+
			" shell in the panel hands it the terminal: %q", got)
	}
}
