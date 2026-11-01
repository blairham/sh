// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
	"sync"
	"syscall"
)

// A pipeline that ends with its last element, and the elements it leaves
// running.
//
// In ksh93 a pipeline ends as soon as its last element has ended, and an
// earlier element that is still running goes on running while the shell runs
// the next command. See Semantics.PipelineEndsWithItsLastElement for the
// measurements and for the cases where ksh93 waits for every element all the
// same.
//
// Every element but one runs on a goroutine here, on a clone of the shell, so
// leaving one running is a goroutine that outlives runPipeline. Four things
// keep that from costing more than it does in ksh93, where the element is a
// process of its own.
//
// **It stops touching the shell the moment the pipeline has ended.** An
// element's end is something the shell notices — a reaped child, a signal held
// for one — and an element that ends while the pipeline is still running tells
// the shell so on its own goroutine, as every element always has. One that ends
// after the pipeline has gone tells it nothing, because the shell is running
// other commands by then and its fields are not the element's to write. The
// decision and the notice are taken under one lock, so an element either ends
// in time to be noticed or is left behind, never both. See pipelineTail.
//
// **It holds nothing of the shell's but descriptors.** A process ksh93 forks
// holds its own copies of the shell's descriptors, so a redirection the shell
// ends, or a stream the shell goes on writing, is nothing to it. An element
// here takes its own copy of its input, its error stream and the rest of the
// table — see ownDescriptors — and writes its output into its own pipe. That
// is only a copy where the stream is a descriptor: an embedder's io.Writer, or
// the value a command substitution is collecting, has none to copy, and an
// element left writing into one would be a second writer the shell and the
// one reading the value know nothing about. So where anything the elements
// inherit is not a descriptor, the pipeline waits for every element, as it
// did before. Measured 2026-11-01 on ksh93u+, that is also where ksh93 is
// seen to wait: `x=$( { { sleep 0.3; echo first >&2; } | :; echo after; }
// 2>&1 )` holds `first` before `after`, where the same pipeline writing to
// the terminal from inside `$( … )` does not hold up the substitution.
//
// **A bare `wait` waits for it.** ksh93 makes the elements it left running
// children the shell can wait for: `{ sleep 0.3; echo first >&2; } | :; wait;
// echo after` writes `first` before `after`. A `wait` with operands does not
// reach them, and nor does a `jobs` listing; see the axis for what ksh93 does
// there.
//
// **The shell at the top waits for it when it ends.** ksh93 exits without
// waiting, and the element, a process, writes after the shell has gone. Here
// the element would end with the process, and its output with it — the
// boundary docs/spec/semantics.md records for a `&` job (#519). A pipeline did
// not lose that output before this, because it waited, so the shell joins the
// elements it left behind once its EXIT trap has run: the output is ksh93's,
// in ksh93's order, and the process exits when it exited before. The same
// goes for `exec` and for the program a command string ends on, which this
// shell becomes only once the elements it left have ended.

// strayElements is every set of elements a pipeline left running, for a shell
// and all its clones.
type strayElements struct {
	mu   sync.Mutex
	list []strayElement
}

// strayElement is the elements one pipeline left running: the runner that ran
// the pipeline, whose bare `wait` waits for them, and the channel closed once
// the last of them has ended.
type strayElement struct {
	owner *Runner
	done  <-chan struct{}
}

// add records a pipeline's elements, and lets go of every record whose
// elements have all ended, so that a loop of such pipelines keeps only the
// ones still running.
func (s *strayElements) add(owner *Runner, done <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.list[:0]
	for _, e := range s.list {
		select {
		case <-e.done:
		default:
			kept = append(kept, e)
		}
	}
	clear(s.list[len(kept):])
	s.list = append(kept, strayElement{owner: owner, done: done})
}

// of is the channels of the elements owner left running.
func (s *strayElements) of(owner *Runner) []<-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []<-chan struct{}
	for _, e := range s.list {
		if e.owner == owner {
			out = append(out, e.done)
		}
	}
	return out
}

// anyRunning reports whether any element in the list, whoever left it, is
// still running. A nil list holds none.
func (s *strayElements) anyRunning() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.list {
		select {
		case <-e.done:
		default:
			return true
		}
	}
	return false
}

// take empties the list and hands back every channel that was in it.
func (s *strayElements) take() []<-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]<-chan struct{}, len(s.list))
	for i, e := range s.list {
		out[i] = e.done
	}
	s.list = nil
	return out
}

// leaveElementsBehind records the elements a pipeline this shell ran has left
// running. The list is made on the parent when a clone is made, so it is
// there by the time a pipeline gets here; it is made here only for a runner
// that came by its dialect after its last clone.
func (r *Runner) leaveElementsBehind(done <-chan struct{}) {
	if r.strays == nil {
		r.strays, r.ownsStrays = &strayElements{}, true
	}
	r.strays.add(r, done)
}

// join waits for every element in the list, whoever left it. Looped,
// because an element can run a pipeline of its own that leaves one. A nil
// list holds none.
func (s *strayElements) join() {
	if s == nil {
		return
	}
	for {
		pending := s.take()
		if len(pending) == 0 {
			return
		}
		for _, done := range pending {
			<-done
		}
	}
}

// joinStrayElements waits for every element the shell and its clones left
// running, on the runner that owns the list: the shell at the top, as it
// ends.
func (r *Runner) joinStrayElements() {
	if r.ownsStrays {
		r.strays.join()
	}
}

// waitForStrayElements is a bare `wait` for the elements this shell's own
// pipelines left running. It reports the trapped signal that cut it short,
// if one did, as a wait for a job does.
func (r *Runner) waitForStrayElements() (syscall.Signal, bool) {
	if r.strays == nil {
		return 0, false
	}
	for _, done := range r.strays.of(r) {
		if sig, hit, _ := r.awaitOrTrap(done, nil); hit {
			return sig, true
		}
	}
	return 0, false
}

// streamsAreDescriptors reports whether everything a pipeline's elements
// inherit from this shell to read and write is a descriptor, or holds no
// state at all: the input, the error stream and every entry of the table.
// The elements' own output is their pipe. See the section above.
func (r *Runner) streamsAreDescriptors() bool {
	switch r.stdin().(type) {
	case *os.File, emptyReader, closedFd:
	default:
		return false
	}
	switch w := r.stderr(); w.(type) {
	case *os.File, closedFd:
	default:
		if w != io.Discard {
			return false
		}
	}
	for _, v := range r.fds {
		switch v.(type) {
		case *os.File, closedFd:
		default:
			return false
		}
	}
	return true
}

// pipelineTail is what a pipeline that may end with its last element keeps
// about the elements before it, under one lock: how many are still running,
// which have ended, and whether the pipeline has gone and left them.
type pipelineTail struct {
	mu       sync.Mutex
	running  int
	ended    []bool
	detached bool
	// shared says an element could not be given its own copy of its error
	// stream, so the pipeline waits for every element. Written while the
	// elements are being made, before any starts.
	shared bool
	// done is closed by the last earlier element to end, once the pipeline
	// has left them: it is what a bare `wait` and the shell's end wait on.
	done chan struct{}
	// lastEnded is closed when the last element ends, where it runs on a
	// goroutine rather than on the shell. nil where the shell runs it.
	lastEnded chan struct{}
}

func newPipelineTail(n int, lastOnAGoroutine bool) *pipelineTail {
	t := &pipelineTail{running: n - 1, ended: make([]bool, n-1), done: make(chan struct{})}
	if lastOnAGoroutine {
		t.lastEnded = make(chan struct{})
	}
	return t
}

// reaped is an earlier element's end as the shell notices it: on the shell,
// as for any element, unless the pipeline has already ended and left it, when
// the shell has moved on and is nobody's to write.
func (t *pipelineTail) reaped(r *Runner) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.detached {
		r.childReapedWithoutNoticing()
	}
}

// handedOver records that an earlier element's goroutine is finished, however
// it ended. An element left behind reports its own error, as a background job
// does, since there is nowhere left to return it to.
func (t *pipelineTail) handedOver(i int, sub *Runner, err error) {
	t.mu.Lock()
	t.ended[i] = true
	t.running--
	detached, last := t.detached, t.running == 0
	t.mu.Unlock()
	if !detached {
		return
	}
	if err != nil {
		sub.diagf("%v\n", err)
	}
	if last {
		close(t.done)
	}
}

// leave decides, once the last element has ended, whether the pipeline ends
// now: where an earlier element is still running and neither the monitor nor
// pipefail asks for all of them. Read here rather than when the pipeline
// started, because the last element may be the shell itself and may have
// moved either.
//
// It hands back what the pipeline reports in place of the slices the
// elements write: an element left running has no status yet and is recorded
// as 0, which nothing can read in the one dialect that leaves one — ksh93 has
// no name for the record.
func (t *pipelineTail) leave(
	r *Runner,
	statuses []int,
	signals []syscall.Signal,
	errs []error,
) ([]int, []syscall.Signal, []error, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running == 0 || t.shared || r.monitor || r.pipefail {
		return nil, nil, nil, false
	}
	t.detached = true
	n := len(statuses)
	st, sigs, es := make([]int, n), make([]syscall.Signal, n), make([]error, n)
	for i := range n {
		if i == n-1 || t.ended[i] {
			st[i], sigs[i], es[i] = statuses[i], signals[i], errs[i]
		}
	}
	return st, sigs, es, true
}
