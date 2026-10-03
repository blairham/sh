// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/syntax"
)

// noticeGrace bounds how long a notice waits for a forked element to settle.
const noticeGrace = 50 * time.Millisecond

// A backgrounded pipeline listed an element at a time.
//
// One dialect's `jobs` writes `a | b &` as a row per element, each in its
// own state: measured 2026-10-02 on zsh 5.9.2 under `-f -c`,
//
//	/bin/sleep 0.3 | cat & jobs
//	        [1]  + running    /bin/sleep 0.3 |·
//	               running    cat
//	/bin/sleep 0.3 | /bin/sleep 0.6 & /bin/sleep 0.45; jobs
//	        [1]  + done       /bin/sleep 0.3 |·
//	               running    /bin/sleep 0.6
//	exit 3 | /bin/sleep 0.3 & /bin/sleep 0.1; jobs
//	        [1]  + exit 3     exit 3 |·
//	               running    /bin/sleep 0.3
//
// where `·` is the blank the bar is followed by. Each element is written the
// way the dialect writes a job's command — `{ /bin/sleep 0.3;:; } | ( cat )`
// lists as `{ /bin/sleep 0.3; :; } |` over `( cat; )` — and `jobs -l` puts
// each element's own process id on its row, where `jobs -p` writes the
// group's on the first and blanks of the same width under it. `${jobtexts}`
// still holds the pipeline as one string.
//
// A pipeline that is not the job's whole command is one row: `{ a | b } &`
// lists as the brace group. See Diagnostics.JobElementLine (#5322).

// jobElement is one element of a backgrounded pipeline, as a listing writes
// it: its command, the first process it started, and how it ended.
type jobElement struct {
	text   string
	pid    int
	ended  bool
	status int
	sig    syscall.Signal
	// shown says the shell has noticed the end, which is what a listing
	// writes: an element that has ended reads `running` until then. See
	// Runner.noticeElementEnds.
	shown bool
	// reaped says the end has been noticed by a wait rather than only by
	// the fork of a later element, which is the one `${jobstates}` shows:
	// measured, `true | /bin/sleep 0.3 & print ${(kv)jobstates}; jobs`
	// writes the `true` running in the parameter and done in the listing.
	reaped bool
	// settled is closed once the element has a process, waits on something
	// outside the shell, or has ended — what a notice waits for, since an
	// element here is a goroutine that can be behind the shell.
	settled chan struct{}
}

// newJobElements makes n elements, each with its settling channel.
func newJobElements(n int) []jobElement {
	elems := make([]jobElement, n)
	for i := range elems {
		elems[i].settled = make(chan struct{})
	}
	return elems
}

// elementSettled closes an element's settling channel.
func (j *Job) elementSettled(i int) {
	j.elemMu.Lock()
	defer j.elemMu.Unlock()
	if i < len(j.elements) {
		j.elements[i].settle()
	}
}

// noticeElementsBefore is the shell noticing which elements before the n-th
// have ended, as it does when it forks the next one. Measured 2026-10-02 on
// zsh 5.9.2: `true | true | /bin/sleep 0.3 & jobs` lists both `true`s done,
// where `/bin/sleep 0.3 | true & jobs` lists the `true` running — nothing was
// forked after it.
func (j *Job) noticeElementsBefore(n int) {
	j.noticeElements(n, false)
}

// noticeElements waits, boundedly, for the first n elements to settle, and
// then shows the ends of those that have ended. A builtin blocked writing
// into a pipe nothing reads yet never settles, so the wait has a limit.
func (j *Job) noticeElements(n int, reaped bool) {
	j.elemMu.Lock()
	chans := make([]chan struct{}, 0, n)
	for i := 0; i < n && i < len(j.elements); i++ {
		chans = append(chans, j.elements[i].settled)
	}
	j.elemMu.Unlock()
	deadline := time.After(noticeGrace)
	for _, c := range chans {
		select {
		case <-c:
		case <-deadline:
		}
	}
	j.elemMu.Lock()
	for i := 0; i < n && i < len(j.elements); i++ {
		if e := &j.elements[i]; e.ended {
			e.shown = true
			e.reaped = e.reaped || reaped
		}
	}
	j.elemMu.Unlock()
}

// trackElements arms a job whose command is a pipeline of several elements
// for a listing that writes one row per element, in the dialect that does.
func (r *Runner) trackElements(j *Job, st *syntax.Stmt) {
	if r.diag().JobElementLine == "" {
		return
	}
	p, ok := st.Expr.(*syntax.Pipeline)
	if !ok || len(p.Cmds) < 2 {
		return
	}
	j.elementsOf = p
	j.elements = newJobElements(len(p.Cmds))
	for i, c := range p.Cmds {
		j.elements[i].text = r.jobElementText(st, c)
	}
}

// jobElementText is one element's command as the dialect writes a job's.
func (r *Runner) jobElementText(st *syntax.Stmt, c syntax.Command) string {
	single := &syntax.Stmt{Expr: &syntax.Pipeline{Cmds: []syntax.Command{c}}}
	text := r.jobCommandText(single)
	if text == "" {
		// A dialect that keeps the typed text and not a reprint: the
		// element's own slice of it.
		if src := st.Text; src != "" {
			text = src
		}
	}
	return strings.TrimSpace(text)
}

// elementStarted records an element's first process, which is the id `jobs -l`
// writes on its row.
func (j *Job) elementStarted(i, pid int) {
	j.elemMu.Lock()
	defer j.elemMu.Unlock()
	if i < len(j.elements) && j.elements[i].pid == 0 {
		j.elements[i].pid = pid
	}
	if i < len(j.elements) {
		j.elements[i].settle()
	}
}

// settle closes the element's settling channel, once. Called with elemMu
// held.
func (e *jobElement) settle() {
	if e.settled == nil {
		return
	}
	select {
	case <-e.settled:
	default:
		close(e.settled)
	}
}

// elementEnded records how an element ended.
func (j *Job) elementEnded(i, status int, sig syscall.Signal) {
	j.elemMu.Lock()
	defer j.elemMu.Unlock()
	if i < len(j.elements) {
		e := &j.elements[i]
		e.ended, e.status, e.sig = true, status, sig
		e.settle()
	}
}

// elementsNow is a copy of the elements, for a listing to read.
func (j *Job) elementsNow() []jobElement {
	j.elemMu.Lock()
	defer j.elemMu.Unlock()
	if len(j.elements) == 0 {
		return nil
	}
	out := make([]jobElement, len(j.elements))
	copy(out, j.elements)
	return out
}

// tracksPipeline says p is the pipeline this job lists an element at a time.
func (j *Job) tracksPipeline(p *syntax.Pipeline) bool {
	return j != nil && j.elementsOf != nil && j.elementsOf == p
}

// printJobElements writes a job's listing a row per element, where it is a
// pipeline the dialect lists that way, and reports whether it did.
func (r *Runner) printJobElements(n int, j *Job, showBg bool, form jobsForm) bool {
	elems := j.elementsNow()
	if len(elems) == 0 || (!showBg && !j.Stopped) {
		return false
	}
	dg := r.diag()
	// One state column for all of a job's rows, as wide as its widest word:
	// measured, a `terminated` first element widens the `running` under it
	// by the same amount.
	states := make([]string, len(elems))
	width := 0
	for i, e := range elems {
		states[i] = r.elementState(j, e)
		width = max(width, len(states[i]))
	}
	for i, e := range elems {
		text := e.text
		if i < len(elems)-1 {
			text += dg.JobElementJoin
		}
		state := states[i] + strings.Repeat(" ", width-len(states[i]))
		pid := e.pid
		if pid == 0 {
			pid = j.Ident()
		}
		switch {
		case i == 0 && form == jobsStateRow:
			r.printf("%s\n", Wording(dg.JobLine, "[%[1]d]%[2]s  %-24[3]s%[4]s", n, r.jobMarker(j), state, text))
		case i == 0:
			r.printf("%s\n", Wording(dg.JobLineLong, "[%[1]d]%[2]s %[3]d %-24[4]s%[5]s",
				n, r.jobMarker(j), pid, state, text))
		case form == jobsStateRow:
			r.printf("%s\n", Wording(dg.JobElementLine, "%-24[1]s%[2]s", state, text))
		case form == jobsGroupRow:
			blank := strings.Repeat(" ", len(strconv.Itoa(r.elementGroupID(j, elems))))
			r.printf("%s\n", Wording(dg.JobElementLineLong, "%[1]s %-24[2]s%[3]s", blank, state, text))
		default:
			r.printf("%s\n", Wording(dg.JobElementLineLong, "%[1]s %-24[2]s%[3]s", strconv.Itoa(pid), state, text))
		}
	}
	return true
}

// elementGroupID is the id `jobs -p` writes on a pipeline's first row: the
// group's, which is its first element's process.
func (r *Runner) elementGroupID(j *Job, elems []jobElement) int {
	if elems[0].pid != 0 {
		return elems[0].pid
	}
	return j.Ident()
}

// elementState is one element's state column: the job's own while it is
// stopped, and otherwise how the element itself has ended, worded as a job's
// end is.
func (r *Runner) elementState(j *Job, e jobElement) string {
	return r.elementStateAs(j, e, e.shown)
}

// elementStateAs is elementState with the end shown or not.
func (r *Runner) elementStateAs(j *Job, e jobElement, shown bool) string {
	if j.Stopped || !shown {
		if j.Stopped {
			return r.jobState(j, false)
		}
		return Wording(r.diag().JobRunning, "Running")
	}
	done := make(chan struct{})
	close(done)
	return r.jobState(&Job{done: done, Status: e.status, EndSig: int(e.sig)}, false)
}

// noticeElementEnds is the shell noticing the elements that have ended, which
// it does where it waits on a child or starts a job — not between two
// builtins. Measured 2026-10-02 on zsh 5.9.2 under `-f -c`, with the first
// element long finished:
//
//	true | { jobs }                     [1]    running    true
//	true | { kill -0 $$; jobs }         running
//	true | { /bin/sleep 0.1; jobs }     done
//	true | { : $(true); jobs }          done
//	true | { (true); jobs }             done
//	true | { /bin/sleep 0.1 & jobs }    done
//	true | { zselect -t 10; jobs }      done
func (r *Runner) noticeElementEnds() { r.noticeElementEndsAs(true) }

// noticeElementEndsAs is noticeElementEnds, told whether the ends are reaped
// — a wait — or only seen, as starting a job sees them: measured 2026-10-02
// on zsh 5.9.2, `echo | { /bin/sleep 0.3 & print ${(kv)jobstates} }` writes
// the `echo` running where `jobs` in the same place writes it done.
func (r *Runner) noticeElementEndsAs(reaped bool) {
	for _, j := range r.jobs {
		if j == nil || j.elementsOf == nil {
			continue
		}
		// A forked element of ours is a goroutine and not a process, so one
		// that runs nothing but builtins can be behind the shell that is
		// about to notice it: each is given until it settles.
		j.noticeElements(len(j.elements), reaped)
		j.elemMu.Lock()
		all := reaped && j.fgPipeline && j.elemsRunning == 0
		j.elemMu.Unlock()
		if all {
			// The job's own status is not the pipeline's, which is still
			// running its last element: measured, `exit 4 | { wait %1 }` is
			// 0.
			j.finish(0)
		}
	}
}

// The forked elements of a pipeline whose last element the shell runs itself
// are a job of their own while that element runs, in the dialect that does —
// and listed, the same element at a time, where the element holds a job slot
// of its own. Measured 2026-10-02 on zsh 5.9.2 under `-f -c`:
//
//	true | { /bin/sleep 0.3 & jobs }        [1]  - done       true
//	                                        [2]  + running    /bin/sleep 0.3
//	/bin/sleep 1 & true | { /bin/sleep 0.3 & jobs }
//	                                        [1], [2]  - done true, [3]  +
//	echo | cat | { jobs }                   [1]    done       echo |·
//	                                               running    cat
//	f() { jobs }; true | f                  [1]    running    true
//	true | jobs                             nothing: a builtin holds no slot
//	true | { jobs %1; echo $?; jobs %% }    the row, 0, then no current job
//	true | { /bin/sleep 0.1; jobs; jobs; wait %1 }
//	                                        [1]    done       true, then
//	                                        nothing, then %1: no such job
//	/bin/sleep 0.3 | { wait %1; echo $? }   0, once the sleep is done
//	exit 4 | { /bin/sleep 0.1; wait %1; echo $? }
//	                                        0
//	true | { /bin/sleep 1 & }; jobs %-      no previous job: the job is gone
//
// So it takes the number the element's command would have held as its slot,
// is never the current job, and once a listing has shown it ended it is
// forgotten, as an ended job is. See Runner.noticeElementEnds for when an
// element's end is shown (#5322).

// pipelineJob builds the job a pipeline's forked elements make, in the
// dialect that lists one, or nothing.
func (r *Runner) pipelineJob(p *syntax.Pipeline) *Job {
	if r.diag().JobElementLine == "" || r.sem().ACommandHoldsAJobSlot != Yes || r.bg != nil {
		return nil
	}
	n := len(p.Cmds)
	j := &Job{
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
		started:    make(chan struct{}),
		stopNote:   make(chan struct{}),
		ident:      r.inventJobIdent(),
		elemsEnded: make(chan struct{}),
		// Each forked element is a part, started once it has a process,
		// waits on something outside the shell, or ends — which is what a
		// notice waits for. See Runner.noticeElementEnds.
		parts:        n - 1,
		fgPipeline:   true,
		elementsOf:   p,
		elements:     newJobElements(n - 1),
		elemsRunning: n - 1,
	}
	st := &syntax.Stmt{Expr: p}
	texts := make([]string, n-1)
	for i, c := range p.Cmds[:n-1] {
		j.elements[i].text = r.jobElementText(st, c)
		texts[i] = j.elements[i].text
	}
	// No process of its own to settle on: `$!` is never this job's.
	j.settleNoPID()
	// The forked elements alone, as `${jobtexts}` holds them: measured,
	// `echo x | cat | { print -r -- ${jobtexts} }` is `echo x | cat`.
	j.Command = strings.Join(texts, " | ")
	return j
}

// pipelineElementEnded records a forked element's end in the pipeline's job,
// and finishes the job with the last of them.
func (j *Job) pipelineElementEnded(i, status int, sig syscall.Signal) {
	j.elementEnded(i, status, sig)
	j.elemMu.Lock()
	j.elemsRunning--
	last := j.elemsRunning == 0
	j.elemMu.Unlock()
	if last {
		// Ended, and not yet finished: the shell learns that where it notices
		// — see Runner.noticeElementEnds — or where a `wait` for it returns.
		close(j.elemsEnded)
	}
}

// waitChannel is what a `wait` for this job blocks on: the end of its
// processes, which for a pipeline's forked elements comes before the shell
// has noticed it.
func (j *Job) waitChannel() <-chan struct{} {
	if j.fgPipeline {
		return j.elemsEnded
	}
	return j.done
}

// ListedOnceEnded says a listing writes this job after it has ended, which
// is the forked elements of a pipeline the shell runs the last element of:
// `jobs` writes them `done` once, and `${jobstates}` too, where an ended `&`
// job has already left. See Runner.pipelineJob.
func (j *Job) ListedOnceEnded() bool { return j.fgPipeline }

// placePipelineJob puts the job a pipeline's forked elements make into the
// table, under the slot the last element's command has just taken.
func (r *Runner) placePipelineJob(j *Job) {
	j.num = r.commandSlot
	at := len(r.jobs)
	for i, other := range r.jobs {
		if other.num > j.num {
			at = i
			break
		}
	}
	r.jobs = slices.Insert(r.jobs, at, j)
}

// JobProcessState is one process of a job as a listing of its processes
// writes it: the id, and the state word the listing would give it.
type JobProcessState struct {
	PID  int
	Word string
	// Exited is the status a process that has been seen to end left, where
	// it ended by itself and not at 0 — zero otherwise.
	Exited int
}

// ElementStates is a job's processes an element at a time, where it is a
// pipeline listed that way: each element's own id and its own state, worded
// as `jobs` words it. Nil for every other job. Measured 2026-10-02 on zsh
// 5.9.2: after `exit 3 | /bin/sleep 0.3 & /bin/sleep 0.1`, `${jobstates[1]}`
// is `running:+:<pid>=exit 3:<pid>=running`.
func (r *Runner) ElementStates(j *Job) []JobProcessState {
	elems := j.elementsNow()
	if len(elems) == 0 {
		return nil
	}
	out := make([]JobProcessState, len(elems))
	for i, e := range elems {
		pid := e.pid
		if pid == 0 {
			pid = j.Ident()
		}
		out[i] = JobProcessState{PID: pid, Word: r.elementStateAs(j, e, e.reaped)}
		if e.reaped && e.ended && e.sig == 0 {
			out[i].Exited = e.status
		}
	}
	return out
}

// EndedWord is the word a listing gives a job that has ended and is still
// listed — the forked elements of a pipeline, `done` once noticed — and
// false for every other job. See Job.ListedOnceEnded.
func (r *Runner) EndedWord(j *Job) (string, bool) {
	if !j.fgPipeline || !j.Finished() {
		return "", false
	}
	return Wording(r.diag().JobDone, "Done"), true
}
