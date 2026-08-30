// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/blairham/sh/syntax"
)

// lockedWriter serialises writes from concurrently running pipeline elements.
//
// A real shell hands each element a file descriptor and the kernel serialises
// them. An io.Writer supplied by a caller carries no such guarantee — a
// bytes.Buffer shared by two elements is a data race, which the race detector
// found here rather than in anything exotic. The shell creates the
// concurrency, so the shell owns the synchronisation; requiring callers to
// pass thread-safe writers would be a surprising thing to demand of an
// interface that says io.Writer.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// runPipeline runs several commands with their streams joined.
//
// The pipes are real OS pipes rather than in-process ones, so an external
// command is handed a file descriptor instead of having its output copied
// through this process. That matters for more than speed: a program that asks
// whether its output is a terminal, or that seeks, gets a truthful answer.
//
// Every element but the last runs on a copy of the shell's state. That is the
// bash and dash answer — the last element runs in a subshell there too — and
// it is deliberately *not* the ksh93 and zsh one, where the last element runs
// in the current shell so `echo x | read v` sets v. docs/spec/semantics.md
// records that as an axis; this takes the majority until the interpreter
// carries a dialect.
func (r *Runner) runPipeline(ctx context.Context, p *syntax.Pipeline) error {
	n := len(p.Cmds)
	readers := make([]*os.File, n)
	writers := make([]*os.File, n)

	// Element i reads from readers[i] and writes to writers[i]; the ends the
	// shell itself uses are left nil and fall back to the runner's streams.
	for i := 0; i < n-1; i++ {
		pr, pw, err := os.Pipe()
		if err != nil {
			return err
		}
		writers[i] = pw
		readers[i+1] = pr
	}

	// The streams the shell itself supplied are shared by every element that
	// does not have a pipe in their place, so they are guarded for the
	// duration of the pipeline.
	sharedOut := &lockedWriter{w: r.stdout()}
	sharedErr := &lockedWriter{w: r.stderr()}

	var wg sync.WaitGroup
	statuses := make([]int, n)
	errs := make([]error, n)

	for i, cmd := range p.Cmds {
		wg.Add(1)
		go func(i int, cmd syntax.Command) {
			defer wg.Done()
			sub := r.clone()
			sub.Stderr = sharedErr
			sub.Stdout = sharedOut
			if readers[i] != nil {
				sub.Stdin = readers[i]
			}
			if writers[i] != nil {
				// A pipe end is this element's alone, so it needs no guard.
				sub.Stdout = writers[i]
			}
			errs[i] = sub.command(ctx, cmd)
			statuses[i] = sub.status

			// Closing the write end is what tells the next element its input
			// has finished. Without it the pipeline deadlocks, which is the
			// classic way to get this wrong.
			if writers[i] != nil {
				_ = writers[i].Close()
			}
			if readers[i] != nil {
				_ = readers[i].Close()
			}
		}(i, cmd)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	// A pipeline reports its *last* command, not its first failure.
	r.status = statuses[n-1]
	return nil
}
