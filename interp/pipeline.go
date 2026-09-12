// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

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
//
// And to *both* output streams together, which is the same argument carried
// one step further. `Stdout` and `Stderr` are two fields and need not be two
// writers: an embedder that wants what `2>&1` gives hands the same buffer to
// both, and a lock per field is then two locks over one writer — the very
// thing the paragraph above rules out, arrived at from the other side. The
// shell cannot tell whether two io.Writers are one object without comparing
// interface values, which panics for a writer whose type is not comparable,
// so it assumes they are. Serializing stderr behind stdout costs a mutex on
// streams a shell writes a line at a time.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// streamLocks is the locks over the streams the shell was handed, shared by a
// runner and every subshell cloned from it — which is exactly the set of
// runners that can be reading or writing those streams at once.
//
// One for writing rather than one each for stdout and stderr: see
// lockedWriter, where the reason is the whole point of the type.
type streamLocks struct {
	write, in sync.Mutex
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
func (r *Runner) lockedStdout() io.Writer {
	return lockWriter(&r.streamLocks().write, r.stdout())
}

func (r *Runner) lockedStderr() io.Writer {
	return lockWriter(&r.streamLocks().write, r.stderr())
}

// lockWriter guards a stream, and hands back a guard that is already there.
//
// Wrapping unconditionally would be a `Write` that takes one mutex twice, and
// a sync.Mutex is not reentrant, so the second call would wait for a lock its
// own goroutine holds and never come back. It is reachable as soon as a
// stream stays guarded past the construct that guarded it — a background job
// leaves the shell's own streams wrapped for as long as the job can write to
// them, and a pipeline in the same shell afterwards would wrap them again.
//
// A *os.File is left alone, which lockedStdin has always done for reads and
// this had to learn for writes. Two goroutines writing one os.File are
// serialized by the descriptor's own lock, and two *processes* writing it are
// serialized by the kernel — which is the arrangement a real shell has, and
// the one lockedWriter exists to imitate where a caller's writer cannot
// provide it. So there is nothing there to guard against.
//
// And wrapping one costs something a shell should not spend. os/exec connects
// a child directly to an *os.File and builds a pipe for anything else, so a
// wrapped stream means every later command is handed a pipe — and a child
// asking whether its output is a terminal gets a different answer for the rest
// of the session. Measured before this: after `sleep 0.05 &`, `test -t 1` in a
// child said no, where it said yes on the line before and says yes in all five
// shells of the panel. That was the price #735 recorded for guarding both
// sides of a construct, and it is not one that has to be paid.
func lockWriter(mu *sync.Mutex, w io.Writer) io.Writer {
	if _, ok := w.(*os.File); ok {
		return w
	}
	// A closed descriptor is left alone for the first of those reasons and
	// for one of its own. There is no state behind closedFd to serialize —
	// every write to it fails with the same errno whoever is writing — and
	// the value is also the marker childOut reads to tell a child the number
	// is closed. Wrapped, it is a writer like any other, and `exec 1>&-;
	// /bin/echo hi &` handed the child an ordinary pipe again (#1260).
	if _, ok := w.(closedFd); ok {
		return w
	}
	if guardedBy(mu, w) {
		return w
	}
	return &lockedWriter{mu: mu, w: w}
}

// WriterUnder is a stream wrapper that can say what it wraps.
//
// A shell builds its output out of layers — a capture that keeps a copy of
// what a command printed, a line discipline that is handed back at the first
// byte, and the guard below — and each of them is an io.Writer around the next
// one. The layers are the point; what they must not do is hide each other,
// because two of the questions this package asks about a stream are questions
// about the *chain* and not about whatever happens to be outermost.
//
// Implemented by anything outside this package that wraps one of the Runner's
// streams and leaves it on the Runner. A wrapper that does not implement it is
// not wrong, it is opaque: the chain stops being readable at that layer, and
// whatever is underneath is invisible to the question below.
type WriterUnder interface{ Unwrap() io.Writer }

// Unwrap is the stream this guard is over, so that a later layer can see the
// guard through whatever was put on top of it. See guardedBy.
func (l *lockedWriter) Unwrap() io.Writer { return l.w }

// guardedBy reports whether this lock is already taken somewhere down a
// stream's chain of wrappers, so that it is not taken a second time.
//
// This is the question lockWriter's doc comment above says it is asking, and
// asking it of the outermost layer alone is what it used to do. That was
// enough for as long as a guard was always outermost, and it stopped being
// true the moment something else wrapped a stream and left the wrapper there:
// the editor's line discipline puts a layer over the Runner's streams for the
// duration of a descriptor handler, and a background job started *inside* one
// of those handlers then wrapped the layer rather than recognizing the guard
// beneath it. One more handler and one more job stacked a second pair, and the
// first write through the result took one mutex twice on one goroutine and
// never came back — an interactive session that reached its first prompt,
// deadlocked writing a diagnostic, and read nothing anybody typed (#2069).
//
// The answer is the chain and not the layer. A guard found anywhere under here
// already serializes every write that reaches it, so the outer layers run
// unserialized and the bytes are still ordered where it matters — which is the
// same bargain lockWriter makes for an *os.File, where the kernel is the guard
// and the layers above it are nobody's to serialize.
func guardedBy(mu *sync.Mutex, w io.Writer) bool {
	for w != nil {
		if l, ok := w.(*lockedWriter); ok && l.mu == mu {
			return true
		}
		u, ok := w.(WriterUnder)
		if !ok {
			return false
		}
		w = u.Unwrap()
	}
	return false
}

// lockedStdin is the shell's input, guarded.
//
// A *os.File is left alone: os/exec hands a file to the child directly and
// copies nothing, so there is no goroutine to race and wrapping it would
// *create* the copying it is meant to make safe.
func (r *Runner) lockedStdin() io.Reader { return r.lockReader(r.In()) }

// lockReader is the same guard over a named stream rather than over the
// shell's current one. It exists because a process substitution's body may be
// handed the input the shell had *before* a pipeline's pipe replaced it, which
// is a different stream from r.In() and needs the guard just as much — two
// readers of one io.Reader is what the guard is for, whichever of the shell's
// inputs they happen to share. See Runner.shellStdin.
func (r *Runner) lockReader(in io.Reader) io.Reader {
	if _, ok := in.(*os.File); ok {
		return in
	}
	// And a closed descriptor is left alone for the reasons lockWriter gives
	// on the other side: nothing to serialize, and a marker childIn has to
	// still be able to see.
	if _, ok := in.(closedFd); ok {
		return in
	}
	mu := &r.streamLocks().in
	// A stream already guarded by *this* lock is handed back as it is, which
	// is lockWriter's rule arriving on the reading side and mattering more
	// here: a lockedReader wrapped in a second lockedReader over the same
	// mutex takes it twice on one goroutine, and a sync.Mutex is not
	// reentrant. It becomes reachable the moment a construct guards both
	// sides of a shared input rather than only the concurrent one — two
	// substitutions in one command wrap what the first of them left, and
	// `cat <(cat) <(cat)` would stop on the first read.
	if l, ok := in.(*lockedReader); ok && l.mu == mu {
		return l
	}
	return &lockedReader{mu: mu, r: in}
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
// timing, when non-nil, collects each element's wall time and external CPU
// for a `time` clause whose layout reports per element. One slot per element,
// already sized by the caller, so the goroutines write disjoint slots.
func (r *Runner) runPipeline(ctx context.Context, p *syntax.Pipeline, timing *pipelineTiming) error {
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
	// Beside each status, what ended that element — zero unless a signal
	// did. A status alone cannot say: 143 is `exit 143` as readily as it is
	// SIGTERM, and one shell reports the signal where pipefail substitutes.
	signals := make([]syscall.Signal, n)
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
	releaseFds := make([]func(), last)
	// A backgrounded pipeline is one job made of several processes, and the
	// shell that ran `&` must not move on before they exist: `kill %1` on the
	// next line means all of them. One element settles the job's pid and
	// releases the body's own share of the count, so raising the count for
	// every element here — before any of them runs — is what keeps it from
	// reaching zero in between. See Job.expectPart.
	if r.bg != nil {
		r.bg.expectPart(last)
	}
	for i := 0; i < last; i++ {
		sub := r.clone()
		if r.bg != nil {
			sub.part = &jobPart{job: r.bg}
		}
		// Every element runs at once, so each one's descriptors are its own
		// copies rather than one table's. An element that closes a parked
		// descriptor — `{ exec {a}<&-; } | { read v <&$a; }` — is a real
		// shell closing its own process's name for the file and nobody
		// else's. See ownDescriptors (#2116).
		releaseFds[i] = sub.ownDescriptors()
		// A pipeline element is a subshell whose trap listing survives in a
		// different pair of shells than `( … )` does, so the boundary says
		// what kind it is.
		sub.retagTrapBoundary(trapContextPipeline)
		// And a different question again for the job table, which one
		// dialect answers by the *shape* of the element: `jobs -p | cat`
		// lists the parent's jobs there and `{ jobs -p; } | cat` does not.
		sub.inheritJobs(pipelineJobBoundary(p.Cmds[i]))
		// Each element is its own job component, so the copy running it
		// names it rather than inheriting whatever the shell last ran.
		sub.killed = p.Cmds[i]
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
			// What the pipe replaced is kept, because one dialect's process
			// substitutions read it rather than the pipe. See
			// Runner.shellStdin for the whole of why, and procSub for the
			// axis that decides whether anything looks.
			sub.shellStdin = r.stdin()
			sub.Stdin = readers[i]
		}
		if writers[i] != nil {
			// A pipe end is this element's alone, so it needs no guard.
			sub.Stdout = writers[i]
		}
		if timing != nil {
			// The element's externals bill its own slot, wherever inside
			// it they run — the clone hands the pointer down.
			sub.elemCPU = &timing.elems[i].cpu
		}
		subs[i] = sub
	}

	for i, cmd := range p.Cmds[:last] {
		wg.Add(1)
		// A failing status until this element has one of its own, so an
		// element that stops without finishing is not read as having
		// succeeded. It is the pipeline's whole answer when it is the last
		// element, and pipefail's when it is any of them, and the zero value
		// of the slice is 0 — so an interpreter bug caught outside this
		// package would otherwise be reported to the script as a pipeline
		// that worked.
		statuses[i] = internalErrorStatus
		r.spawn(func() {
			start := time.Now()
			errs[i] = subs[i].command(ctx, cmd)
			if timing != nil {
				timing.elems[i].wall = time.Since(start)
			}
			statuses[i] = subs[i].status
			signals[i] = subs[i].diedOfSig
		}, func() {
			// Done last, because it is what releases the shell to read
			// everything above.
			defer wg.Done()
			// An element that ended without ever starting a process is as
			// started as it is going to get, and the shell that ran `&` is
			// waiting on exactly that.
			subs[i].part.started()
			// An element that never reached a trace point must still let the
			// next one print, or the pipeline deadlocks on its own logging.
			subs[i].releaseTraceTurn()

			// Closing the write end is what tells the next element its input
			// has finished. Without it the pipeline deadlocks, which is the
			// classic way to get this wrong — and however this element
			// ended, because the element downstream is reading and the shell
			// is waiting for both.
			if writers[i] != nil {
				_ = writers[i].Close()
			}
			if readers[i] != nil {
				_ = readers[i].Close()
			}
			// And this element's copies of the table, which nothing after
			// the element is entitled to.
			releaseFds[i]()
		})
	}
	if inCurrent {
		// The last element runs here, on the shell itself, so what it
		// assigns survives the pipeline.
		i := n - 1
		if gates != nil {
			r.traceWait, r.traceDone, r.traceOnce = gates[i-1], gates[i], &sync.Once{}
			defer func() { r.traceWait, r.traceDone, r.traceOnce = nil, nil, nil }()
		}
		// In a scope of its own so its defer runs *here* and not at the end
		// of the pipeline. What it puts back is the shell's own — this
		// element runs on the shell rather than on a copy — and it has to be
		// put back before the wait below: the element upstream may be
		// blocked writing into the pipe this one was reading, and closing
		// that reader is what releases it. A cleanup deferred to the end of
		// the function would sit behind the wait for the goroutine it is
		// what releases.
		//
		// Deferred rather than written straight through for the reason
		// everything else in this file is: a bug in the element leaves a
		// session with a standard output that is a closed pipe, and the
		// element upstream writing into one nothing will ever read.
		func() {
			savedIn, savedOut, savedErr := r.Stdin, r.Stdout, r.Stderr
			savedCPU := r.elemCPU
			savedShellIn := r.shellStdin
			defer func() {
				r.Stdin, r.Stdout, r.Stderr = savedIn, savedOut, savedErr
				r.elemCPU = savedCPU
				// Put back with the streams, and for a sharper reason than
				// tidiness: this element runs on the shell itself, so a
				// record left behind would outlive the pipeline and tell
				// every later substitution to read a pipe that is closed.
				r.shellStdin = savedShellIn
				if readers[i] != nil {
					_ = readers[i].Close()
				}
			}()
			if readers[i] != nil {
				r.shellStdin = r.stdin()
				r.Stdin = readers[i]
			}
			r.Stdout, r.Stderr = sharedOut, sharedErr
			if timing != nil {
				r.elemCPU = &timing.elems[i].cpu
			}
			// No naming needed here: this element runs on the shell itself
			// rather than on a copy, so the dispatch records it the way it
			// records any other command. Verified by mutation, not assumed.
			start := time.Now()
			errs[i] = r.command(ctx, p.Cmds[i])
			if timing != nil {
				timing.elems[i].wall = time.Since(start)
			}
			statuses[i] = r.status
			signals[i] = r.diedOfSig
		}()
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
		chosen := -1
		for i, st := range statuses {
			if st != 0 {
				r.status, chosen = st, i
			}
		}
		if chosen >= 0 && chosen != n-1 {
			// An element pipefail went looking for, rather than the one the
			// pipeline reports anyway. Where it died of a signal, one shell
			// gives the number alone.
			r.status = r.substitutedSignalStatus(r.status, signals[chosen])
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
		args := r.afterPrecommands(x.Args)
		if len(args) == 0 {
			// Assignments with no command name, which persist by definition.
			return len(x.Assigns) > 0
		}
		name := literalName(args[0])
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

// substitutedSignalStatus is what a pipeline reports for an element pipefail
// chose over its last one, where that element died of a signal.
//
// The status a signal death produces is the shell's own — 128 or 256 plus the
// number, see signalDeathStatus — and one shell in the panel does not use it
// here. Measured, its `set -o pipefail` reports 13 for SIGPIPE and 15 for
// SIGTERM where the same death anywhere else in that shell reports 269 and
// 271: a foreground command, a subshell, a command substitution, a `wait` and
// the pipeline's *last* element all keep the ordinary encoding.
//
// So this is about the substitution and not about pipelines or about signals
// in general, which is why it is its own axis rather than a second reading of
// the one that chooses the base. An ordinary non-zero exit is substituted
// unchanged everywhere, so a signal is the whole of the difference.
func (r *Runner) substitutedSignalStatus(st int, sig syscall.Signal) int {
	if sig == 0 {
		return st
	}
	if r.ask(r.sem().PipefailSubstitutesTheBareSignal,
		"the status pipefail substitutes for an element a signal killed") {
		return int(sig)
	}
	if r.sem().PipefailSubstitutesTheBareSignal == Unspecified {
		// The refusal is the answer, and ask has already chosen the status
		// that goes with it. Reporting the element's own here would let a
		// core with no dialect quietly pick one of the two conventions.
		return r.status
	}
	return st
}
