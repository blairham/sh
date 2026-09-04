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

// lockedWriter serializes writes to a stream the shell was handed.
//
// A real shell hands each element a file descriptor and the kernel serializes
// them. An io.Writer supplied by a caller carries no such guarantee — a
// bytes.Buffer shared by two elements is a data race, which the race detector
// found here rather than in anything exotic. The shell creates the
// concurrency, so the shell owns the synchronization; requiring callers to
// pass thread-safe writers would be a surprising thing to demand of an
// interface that says io.Writer.
//
// The lock is a pointer to one held by the shell, and not a field of its own.
// Three things write concurrently to these streams — a pipeline's elements, a
// background job, a process substitution — and a lock per writer would give
// each of them a *different* lock over the *same* io.Writer, which excludes
// nothing. The lock has to belong to the stream.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// streamLocks is one lock per stream the shell was handed, shared by a runner
// and every subshell cloned from it — which is exactly the set of runners that
// can be reading or writing those streams at once.
type streamLocks struct {
	out, err, in sync.Mutex
}

// lockedReader serializes reads from a stream the shell was handed, for the
// same reason lockedWriter serializes writes.
//
// A real shell's standard input is a file descriptor, and two children
// reading it race for bytes in the kernel, which is arbitrary and safe. An
// io.Reader from a caller has no such guarantee — os/exec copies from it on a
// goroutine per child — so two children reading it is a data race in this
// process rather than a scramble in the kernel. The shell made the
// concurrency, so the shell guards it.
type lockedReader struct {
	mu *sync.Mutex
	r  io.Reader
}

func (l *lockedReader) Read(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Read(p)
}

// streamLocks returns the shell's locks, making them on first use.
//
// Lazily, because a Runner is a struct literal its caller fills in and there
// is no constructor to do it. Safe to be lazy, and clone() needs no line of
// its own for it, because the lock is always taken from the runner that
// *creates* the concurrency — the one running the pipeline, starting the
// background job, or naming the substitution — and that runner is the one
// holding the stream. A subshell that starts its own carries the parent's
// pointer with the struct copy, and one that does not is writing through the
// parent's guarded writer already.
func (r *Runner) streamLocks() *streamLocks {
	if r.streams == nil {
		r.streams = &streamLocks{}
	}
	return r.streams
}

// lockedStdout and lockedStderr are the shell's streams, guarded.
func (r *Runner) lockedStdout() *lockedWriter {
	return &lockedWriter{mu: &r.streamLocks().out, w: r.stdout()}
}

func (r *Runner) lockedStderr() *lockedWriter {
	return &lockedWriter{mu: &r.streamLocks().err, w: r.stderr()}
}

// lockedStdin is the shell's input, guarded.
//
// A *os.File is left alone: os/exec hands a file to the child directly and
// copies nothing, so there is no goroutine to race and wrapping it would
// *create* the copying it is meant to make safe.
func (r *Runner) lockedStdin() io.Reader {
	in := r.In()
	if _, ok := in.(*os.File); ok {
		return in
	}
	return &lockedReader{mu: &r.streamLocks().in, r: in}
}

// runPipeline runs several commands with their streams joined.
//
// The pipes are real OS pipes rather than in-process ones, so an external
// command is handed a file descriptor instead of having its output copied
// through this process. That matters for more than speed: a program that asks
// whether its output is a terminal, or that seeks, gets a truthful answer.
//
// Every element but the last runs on a copy of the shell's state. Whether the
// *last* one does is the LastPipelineElementInCurrentShell axis: dash and bash
// give it a subshell, ksh93 and zsh run it in the current shell so
// `echo x | read v` sets v.
//
// The axis is asked only when the answer could be observed — when the last
// element is a builtin, a function, or a brace group, all of which can touch
// the shell. An external command cannot, so `echo x | cat` needs no answer
// from anyone and the core does not refuse it. That is the same rule the
// `[^…]` axis uses: ask about the construct in front of you, not about every
// construct that shares a code path.
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
	sharedOut := r.lockedStdout()
	sharedErr := r.lockedStderr()

	var wg sync.WaitGroup
	statuses := make([]int, n)
	errs := make([]error, n)

	// Decided before anything starts, because asking from inside a goroutine
	// would interleave the diagnostic with the pipeline's output.
	inCurrent := r.lastElementIsObservable(p.Cmds[n-1]) &&
		r.ask(r.sem().LastPipelineElementInCurrentShell, "the last pipeline element running in the current shell")
	if r.unspecified {
		// No dialect answered, so the pipeline does not run at all. Running
		// it and reporting afterwards would pick a side and say it had not.
		for i := range readers {
			if readers[i] != nil {
				_ = readers[i].Close()
			}
			if writers[i] != nil {
				_ = writers[i].Close()
			}
		}
		r.status = 2
		return nil
	}
	last := n
	if inCurrent {
		last = n - 1
	}

	// The sub-runners are built here, on this goroutine, rather than inside
	// each one. clone() reads the runner's fields, and the last element —
	// when it runs in the current shell — writes them; doing both at once is
	// a race the detector catches and a corrupted stream in production.
	// One gate per element, so element i prints after i-1 has. Only the
	// trace point is ordered; the commands still run at the same time.
	var gates []chan struct{}
	if r.xtrace {
		gates = make([]chan struct{}, n)
		for i := range gates {
			gates[i] = make(chan struct{})
		}
	}
	subs := make([]*Runner, last)
	for i := 0; i < last; i++ {
		sub := r.clone()
		if gates != nil {
			if i > 0 {
				sub.traceWait = gates[i-1]
			}
			sub.traceDone, sub.traceOnce = gates[i], &sync.Once{}
		}
		sub.Stderr = sharedErr
		sub.Stdout = sharedOut
		if i != n-1 {
			// Not the element whose status the pipeline reports, which is
			// what decides whether a signal that ends it is remarked on.
			sub.midPipeline = true
			// Only the last element is the job. A backgrounded pipeline is
			// one job with one pid, and bash and zsh both report the *last*
			// element's — `sleep 1 | cat &` sets `$!` to the `cat`. Every
			// element carrying the job meant every element writing the same
			// Job.PID from its own goroutine: a data race, and wrong for all
			// but one of them.
			sub.bg = nil
		}
		if readers[i] != nil {
			sub.Stdin = readers[i]
		}
		if writers[i] != nil {
			// A pipe end is this element's alone, so it needs no guard.
			sub.Stdout = writers[i]
		}
		subs[i] = sub
	}

	for i, cmd := range p.Cmds[:last] {
		wg.Add(1)
		go func(i int, cmd syntax.Command) {
			defer wg.Done()
			errs[i] = subs[i].command(ctx, cmd)
			statuses[i] = subs[i].status
			// An element that never reached a trace point must still let the
			// next one print, or the pipeline deadlocks on its own logging.
			subs[i].releaseTraceTurn()

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
	if inCurrent {
		// The last element runs here, on the shell itself, so what it
		// assigns survives the pipeline.
		i := n - 1
		if gates != nil {
			r.traceWait, r.traceDone, r.traceOnce = gates[i-1], gates[i], &sync.Once{}
			defer func() { r.traceWait, r.traceDone, r.traceOnce = nil, nil, nil }()
		}
		savedIn, savedOut, savedErr := r.Stdin, r.Stdout, r.Stderr
		if readers[i] != nil {
			r.Stdin = readers[i]
		}
		r.Stdout, r.Stderr = sharedOut, sharedErr
		errs[i] = r.command(ctx, p.Cmds[i])
		statuses[i] = r.status
		r.Stdin, r.Stdout, r.Stderr = savedIn, savedOut, savedErr
		if readers[i] != nil {
			_ = readers[i].Close()
		}
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	// A pipeline reports its *last* command, not its first failure — which
	// is exactly why the others are worth keeping.
	r.recordPipeStatus(statuses)
	r.status = statuses[n-1]
	if r.pipefail && r.status == 0 {
		// Recorded before the status changes, because "only pipefail saw it"
		// means exactly that the last element did not.
		for _, st := range statuses {
			if st != 0 {
				r.pipefailRaised = true
				break
			}
		}
	}
	if r.pipefail {
		// The last failure rather than the first: `false | false | true` is
		// 1 either way, but where two elements fail with different statuses
		// it is the rightmost one that is reported. Measured against bash,
		// ksh93 and zsh, which agree.
		for _, st := range statuses {
			if st != 0 {
				r.status = st
			}
		}
	}
	return nil
}

// lastElementIsObservable reports whether running a command in the current
// shell rather than in a subshell could be seen from outside the pipeline.
//
// An external command has its own process either way, so the axis makes no
// difference to it and nothing is asked. A builtin, a function or a brace
// group can assign a variable or change the directory, and there the answer
// shows.
func (r *Runner) lastElementIsObservable(c syntax.Command) bool {
	switch x := c.(type) {
	case *syntax.SimpleCmd:
		if len(x.Args) == 0 {
			// Assignments with no command name, which persist by definition.
			return len(x.Assigns) > 0
		}
		name := literalName(x.Args[0])
		if name == "" {
			// Not a plain literal, so it could expand to anything; assume it
			// matters rather than quietly picking a side.
			return true
		}
		if _, ok := r.funcs[name]; ok {
			return true
		}
		_, isBuiltin := r.lookupBuiltin(name)
		return isBuiltin
	case *syntax.Subshell:
		// Explicitly a subshell already, so the axis changes nothing.
		return false
	case nil:
		return false
	}
	// A group, loop, conditional or case: all of them can assign.
	return true
}

// literalName reports a word's text when it is a single unquoted literal, and
// "" when it is anything an expansion could change.
func literalName(w *syntax.Word) string {
	if w == nil || len(w.Spans) != 1 {
		return ""
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return ""
	}
	return s.Value
}
