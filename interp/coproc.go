// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/blairham/sh/syntax"
)

// coprocClause runs `coproc [NAME] command`: the command goes to the
// background with a pipe on each of its named streams, and the shell keeps
// the near ends — one to read what the command writes, one to write what it
// reads.
//
// How a script *reaches* those ends is the dialect's, and the two shells with
// the word answer it differently. bash publishes them as the two elements of
// an array, with the process in NAME_PID, and a script writes
// `echo hi >&"${COPROC[1]}"`. zsh publishes nothing at all — it has no name
// for a coprocess and no array — and a script speaks to it with `print -p`
// and `read -p`. Measured 2026-09-05: `coproc cat; print -p hi; read -p l`
// answers `hi` there, and `${COPROC[0]}` is empty. So the ends are kept on
// the runner either way and the array is CoprocEndsInAnArray's question.
//
// Real pipes rather than in-process ones, because the command is usually an
// external process and a pipe an os/exec child inherits is a descriptor, not
// a Go value. Closing the write end through `{v}>&-` is what lets the
// command see its input finish, which is why that close is a real one.
func (r *Runner) coprocClause(ctx context.Context, c *syntax.CoprocClause) error {
	name, ok := r.coprocName(c)
	if !ok {
		return nil
	}
	// A coprocess is a subshell too, and startBeside is where it ends — see
	// concurrentcommand.go, which ends every command run beside the shell in
	// one place rather than once per caller.
	job, err := r.startCoproc(ctx, name, func(sub *Runner) error {
		return sub.command(ctx, c.Cmd)
	})
	if err != nil || job == nil {
		return err
	}
	if r.ask(r.sem().CoprocEndsInAnArray, "a coprocess putting its ends in an array") {
		// The ends are an array, so the name answers the array literal's
		// question rather than the scalar store's: a reference with nothing
		// to point at gives the attribute up, and one aimed at an element
		// has nowhere to put two descriptors. Measured on bash 5.3.20 —
		// `typeset -n x; coproc x { :; }` warns once and fills `x`, and
		// `typeset -n x=A[0]; coproc x { :; }` writes `` `A[0]': not a valid
		// identifier `` and fills nothing. This aimed the reference at the
		// first descriptor and then at the second, so a valueless reference
		// drew the refusal twice and an element target made a parameter
		// whose name had a subscript in it. The coprocess itself still ran
		// and still reports 0, which is bash's answer too.
		target, write := r.namerefArrayLiteralTarget(name)
		if !write {
			// Reported there, and the coprocess itself still ran and still
			// reports 0 — the ends simply reach no name.
			r.status = 0
			return nil
		}
		name = target
		r.setArrayElem(name, 0, "0", itoa(r.coproc.read))
		r.setArrayElem(name, 1, "1", itoa(r.coproc.write))
		r.setVar(name+"_PID", itoa(job.Ident()))
		// Kept so that the reaping can take back exactly what was published.
		// See Semantics.ReapedCoprocessEnds and forgetCoprocNames.
		r.coproc.name = name
	} else if r.unspecified {
		return nil
	}
	r.status = 0
	return nil
}

// coprocName is what this clause's near ends are published under, and reports
// whether the clause runs at all.
//
// The word standing where the name belongs is **expanded here and judged
// here**, which is the whole reason the grammar no longer refuses it. Measured
// 2026-09-17 on bash 5.3.20, script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME:
//
//	v=q; coproc $v { … }      the ends land in q, status 0
//	coproc "q" { … }          the same — quoting is not part of the name
//	v="a b"; coproc $v { … }  `` `a b': not a valid identifier ``, status 1
//	touch zz1 zz2
//	  coproc zz* { … }        `` `zz*': not a valid identifier ``, status 1
//	coproc @ { … }            `` `@': not a valid identifier ``, status 1
//
// So it expands as an assignment's value does — one field, no splitting and
// no globbing — and a result that is not a name is reported, leaves the body
// unrun and carries on. The next line runs and sees 1.
//
// **An expansion that comes to nothing is the one shape not copied.** For
// `coproc ${nosuch} { … }` bash names the target `(null)`, which is its C
// library rendering a pointer it never filled in rather than anything about
// the language; the empty name is reported as the empty name here.
//
// The sentence is written out rather than taken from [Diagnostics], for the
// reason namerefArrayLiteralTarget's identical one is: only a dialect with
// [syntax.Dialect.CoprocName] can reach it, and that is bash alone, so a
// field would be a wording no second shell could ever disagree with.
func (r *Runner) coprocName(c *syntax.CoprocClause) (string, bool) {
	if c.NameWord == nil {
		if c.Name == "" {
			return "COPROC", true
		}
		return c.Name, true
	}
	name := r.expandAssignValue(c.NameWord)
	if isPlainName(name) {
		return name, true
	}
	r.diagf("`%s': not a valid identifier\n", name)
	r.status = 1
	return "", false
}

// coprocStmt runs `cmd |&`, ksh93's spelling of the same construct: a
// statement terminator rather than a word in front of a command, so what goes
// to the background is the whole and-or and there is no name to publish the
// ends under.
//
// **A second one while the first is still running is refused, and fatally.**
// Measured on ksh93u+ 2012-08-01, 2026-09-07, from a script file under
// `env -i`: two `cat |&` in a row answer `process already exists` and the
// script ends at status 1 with the line after the second one unreached. It is
// about a coprocess still *running* and not about one ever having been
// started — `true |&`, a wait, then `cat |&` is accepted, status 0.
//
// That is the opposite of what the `coproc` word does, measured the same day:
// a second `coproc cat` in bash 5.3.15 and in zsh 5.9.2 replaces the first,
// silently, at status 0 and with the script carrying on. Two constructs, two
// answers, both measured — which is why this lives on the operator's path
// rather than becoming a dialect axis over one shared path.
func (r *Runner) coprocStmt(ctx context.Context, st *syntax.Stmt) error {
	if r.coproc != nil && r.coproc.job != nil && !r.coproc.job.Finished() {
		// Named for the statement that was refused, the way a command's own
		// complaints are: nothing has run for this statement yet, so the
		// line the previous command left behind would be the wrong one.
		//
		// Except in the one dialect that has the operator, which sites it at
		// the last statement it entered instead — a number a coprocess
		// statement never advances, so it is behind the operator by however
		// much ran in front of it. See
		// Diagnostics.CoprocessAlreadyRunningNamesTheLastStatementEntered for
		// the thirteen shapes that say so.
		if r.diag().CoprocessAlreadyRunningNamesTheLastStatementEntered {
			r.line = r.lastStatementLine()
		} else {
			r.line = r.lineOf(st.Pos())
		}
		r.fatal("%s\n", Wording(r.diag().CoprocessAlreadyRunning, "process already exists"))
		return nil
	}
	if _, err := r.startCoproc(ctx, st.Text, func(sub *Runner) error {
		return sub.expr(ctx, st.Expr)
	}); err != nil {
		return err
	}
	r.status = 0
	return nil
}

// startCoproc is the machinery both spellings share: a pipe on each of the
// command's named streams, the far ends handed to a background job and the
// near ends kept in the shell's own descriptor table.
//
// The far ends and the goroutine are [Runner.startBeside]'s, which is the same
// arrangement with the pipe-making left out — see concurrentcommand.go, where
// that half was lifted out so a dialect could reach it with a pseudo-terminal
// instead. What stays here is everything about *pipes* and everything about a
// coprocess being a **job**, which a command started through the public seam
// is not: measured 2026-09-20 on zsh 5.9.2, `zpty -b P 'sleep 2'` leaves
// `jobs` empty and `$!` at 0.
//
// It returns the job so the caller that has a name to publish can read its
// process, and a nil job with a nil error when the pipes could not be made —
// which is already reported and already status 1.
func (r *Runner) startCoproc(ctx context.Context, name string, run func(*Runner) error) (*Job, error) {
	// What the command reads: the shell writes shellW, the command reads
	// childIn. And the reverse for what it writes.
	childIn, shellW, err := os.Pipe()
	if err != nil {
		r.diagf("%v\n", err)
		r.status = 1
		return nil, nil
	}
	shellR, childOut, err := os.Pipe()
	if err != nil {
		_ = childIn.Close()
		_ = shellW.Close()
		r.diagf("%v\n", err)
		r.status = 1
		return nil, nil
	}

	// Only the two named streams go through the pipes; complaints still reach
	// whoever is watching the shell, which is what a nil Err means to the
	// seam. The ordering of the closes, the descriptor table the body gets
	// and the handing over of the status are all startBeside's now, and the
	// reasons are written down there.
	job := r.startBeside(ctx, name,
		concurrentStreams{in: childIn, out: childOut, closeEnds: true}, true,
		func(_ context.Context, sub *Runner) error { return run(sub) })

	r.addJob(job)
	r.setLastJob(job)
	r.becomeCurrentJob(job)

	// The near ends go into the descriptor table for keeps, at the numbers
	// the dialect puts them at — see coprocEndNumbers and
	// Semantics.CoprocessEndPlacement.
	// Marked as the shell's own, so they stay out of an external child's
	// descriptor table — see shellOwnedFd for what a child holding the write
	// end open would cost the coprocess.
	rfd, wfd := r.coprocEndNumbers()
	r.setFd(rfd, shellOwnedFd{shellR})
	r.setFd(wfd, shellOwnedFd{shellW})
	// Kept whichever dialect this is: `print -p` and `read -p` need them in
	// the shell that has no array to find them in, and the shell that has one
	// loses nothing by the record. A second `coproc` replaces the first,
	// which is what the shell with the letters does — measured, the second
	// one is the one `print -p` reaches.
	r.coproc = &coprocEnds{read: rfd, write: wfd, job: job, owner: r}
	return job, nil
}

// coprocEndNumbers picks the two numbers the shell's own ends are kept at.
//
// Where they go is the dialect's — Semantics.CoprocessEndPlacement — and the
// two answers are not two bases: one counts *up* from the ordinary allocation
// base, the way `exec {v}>f` does, and the other counts *down* from the top of
// the table so the numbers a script allocates for itself stay clear.
//
// The downward answer takes **four** numbers and keeps the first and the
// fourth, which is bash's own arrangement rather than an accident of ours: it
// moves its read end, the child's output, the child's input and its write end,
// in that order, and closes the child's two in the parent straight after the
// fork. Publishing the first and fourth of the four highest free numbers is
// what reproduces the sequence a run of coprocesses gets — `63 60`, then
// `62 58`, then `61 56`, then `59 54` — and it reproduces the arrangement
// under a script's own parked descriptors too. Here the child's ends are
// files handed to a goroutine and never enter this table at all, so the two
// numbers in the middle are computed and dropped rather than held.
//
// The descent stops at the allocation base for the same reason nextFreeFd
// starts there: below it are the single digits a script addresses by number,
// and a shell with nowhere left above the base takes the ordinary answer
// instead.
func (r *Runner) coprocEndNumbers() (read, write int) {
	if r.sem().CoprocessEndPlacement == CoprocEndsAtTheTopOfTheTable && r.topOfTableIsReachable() {
		if fds, ok := r.highestFreeFds(topOfTheDescriptorTable, 4); ok {
			return fds[0], fds[3]
		}
	}
	read = r.nextFreeFd(-1)
	for write = read + 1; ; write++ {
		if _, held := r.fds[write]; !held {
			return read, write
		}
	}
}

// topOfTheDescriptorTable is the number the downward answer starts at.
//
// A constant rather than a fraction of anything, measured 2026-09-13 on bash
// 5.3.15 by sweeping `ulimit -n` from 20 to 256: the published pair is `63 60`
// at every limit of 64 and above — including the default 1048576 — and the
// ends are not moved at all at any limit of 63 or below.
const topOfTheDescriptorTable = 63

// topOfTableIsReachable says whether this process could hold that number.
//
// It is the condition the sweep above found: bash moves the ends only where 63
// is a legal descriptor, and answers with its raw pipe numbers where it is not.
// Asked of the embedder rather than of the kernel, which is the same route
// refuseFdOverLimit takes — a Runner given no GetRlimit has no limit to be
// asked about, and a library that was given none is not the place to invent
// one.
func (r *Runner) topOfTableIsReachable() bool {
	return r.fdWithinOpenFileLimit(topOfTheDescriptorTable)
}

// fdWithinOpenFileLimit says whether this process could hold the number fd.
//
// The condition topOfTableIsReachable asks about the constant, asked about any
// number — which is what Semantics.SubstitutionEndPlacement's fourth value
// needs, BusyBox counting up from 64 where bash counts down from 63 and both
// giving up on the same test one number apart.
func (r *Runner) fdWithinOpenFileLimit(fd int) bool {
	if r.GetRlimit == nil {
		return true
	}
	soft, _, err := r.GetRlimit(ResourceOpenFiles)
	if err != nil || soft == RlimitInfinity {
		return true
	}
	return int64(fd) < soft
}

// highestFreeFds is the n highest free entries at or below from, in descending
// order, or false where there are not that many above the allocation base.
func (r *Runner) highestFreeFds(from, n int) ([]int, bool) {
	base := r.sem().FirstAllocatedDescriptor.number()
	out := make([]int, 0, n)
	for fd := from; fd >= base && len(out) < n; fd-- {
		if _, held := r.fds[fd]; !held {
			out = append(out, fd)
		}
	}
	if len(out) < n {
		return nil, false
	}
	return out, true
}

// retireCoproc lets go of a coprocess that has ended, in the way the dialect
// says a reaped coprocess's ends are let go of.
//
// **It is called where the shell waits for a child, and nowhere else**, which
// is measured rather than convenient: the notice is not delivered by `wait`
// and it is not delivered by the next command either. Measured 2026-09-12 on
// bash 5.3.15, `coproc CP { echo hi; }` followed by each of these and then by
// `${#CP[@]}`:
//
//	:            2      a hundred builtins, and it is still 2
//	jobs         2      asking after the jobs is not waiting for one
//	( : )        0      a subshell is a fork and a wait
//	: | :        0      so is a pipeline element
//	/usr/bin/…   0      so is an external command
//	wait         0
//
// So the trigger is the shell reaping *anything*, and that is a deterministic
// stand-in for a rule that is not: bash's notice rides on SIGCHLD arriving
// whenever the child gets round to exiting and landing at the next command
// boundary after that, so a construct wins the race by taking long enough
// *and* running commands. Five iterations of `for ((i=0;i<n;i++)); do :;
// done` answer 2 five times running and five hundred answer 0 five times
// running, with no fork in either; `read -t 1` spends a whole second in one
// command and answers 2. The four places here match every shape bash answers
// the same way twice, and the long-loop shape it also answers the same way
// twice is the one they miss — recorded in docs/spec/semantics.md and filed
// as #2468, because following bash there costs the short-loop shape.
//
// Placing it at the top of every statement instead would have retired a
// coprocess before the very next line could read what it wrote — `coproc CP {
// echo hi; }; read -r a <&${CP[0]}` answers `hi` in bash and would have
// answered an ambiguous redirect here.
//
// The main thread is the only thread that runs it. The reaping itself happens
// on the goroutine running the coprocess body, and all this reads of that is
// Job.Finished, which is a closed channel; every table it then edits — the
// descriptors, the variables — belongs to this runner and is touched here
// alone. Doing the unset on the reaping goroutine would be a data race on
// Runner.Vars against whatever the script is doing at the time.
func (r *Runner) retireCoproc() {
	c := r.coproc
	if c == nil || c.owner != r || c.retired || c.job == nil || !c.job.Finished() {
		return
	}
	c.retired = true
	switch r.sem().ReapedCoprocessEnds {
	case CoprocWriteEndGoesWithTheCoprocess:
		// The read end stays: what the coprocess wrote before it ended is
		// still in the pipe, and a script still reads it. It goes when that
		// read finds nothing left — see coprocReadEnded, which is not this
		// axis's to decide.
		r.forgetCoprocFd(c.write)
		c.write = -1
	case CoprocEndsGoWithTheCoprocess:
		r.forgetCoprocFd(c.read)
		r.forgetCoprocFd(c.write)
		r.forgetCoprocNames()
		// Nothing is left to speak to, so the letters that reach a coprocess
		// by name find none running — which is the same refusal they give
		// before any coprocess was started.
		r.coproc = nil
	}
}

// coprocReadEnded is the end-of-file a read of the coprocess's near end found,
// and it is the whole coprocess that goes there rather than only that end.
//
// **Shared ground rather than an axis**, measured 2026-09-12 on both shells
// that have the letters, with a coprocess writing one line and exiting. The
// first `read -p` answers the line, the second is a silent end-of-file at 1,
// and the *third* is `-p: no coprocess` in zsh 5.9.2 and `read: no query
// process` in ksh93 — and so is a `print -p` after it, which is what says the
// write end went with the read one rather than the read end alone. Neither
// shell needs the coprocess reaped first: the same three answers come back
// with no `wait` anywhere and the reaping unobserved.
//
// bash never reaches this, and that is measured too rather than assumed: it
// spells `-p` as a prompt and reads its coprocess through the array, and two
// reads that both find end-of-file leave `${#CP[@]}` at 2. So the array is
// taken back by the reaping alone — see Semantics.ReapedCoprocessEnds — and
// nothing here has to ask which dialect it is in.
//
// Reported by the read rather than noticed here, because end-of-file is not a
// state a descriptor is in: it is what a read came back with, and nothing else
// in this shell is reading that pipe.
func (r *Runner) coprocReadEnded() {
	c := r.coproc
	if c == nil || c.owner != r || c.readEnded {
		return
	}
	c.readEnded = true
	r.forgetCoprocFd(c.read)
	r.forgetCoprocFd(c.write)
	r.coproc = nil
}

// forgetCoprocFd takes one near end out of the descriptor table.
//
// The entry goes and the file does not get closed here, which is deliberate:
// a script may have duplicated the end onto a number of its own with `exec
// 3>&${CP[1]}`, and that duplicate is the same open file. Measured 2026-09-12
// on bash 5.3.15 — after the coprocess is reaped, a write through such a 3 is
// still a write into a pipe with no reader and still ends the shell on
// SIGPIPE, which is exactly what closing the file underneath it would turn
// into a complaint instead. An end nothing else names is unreferenced once
// the entry goes, and the file's own cleanup is what reclaims it.
//
// A number saved out of the array as a *number* is not a duplicate and is not
// protected by any of that: `r=${CP[0]}` and a later `<&$r` finds nothing in
// the table and is `Bad file descriptor`, which is what bash answers.
func (r *Runner) forgetCoprocFd(fd int) {
	if fd < 0 {
		return
	}
	delete(r.fds, fd)
}

// forgetCoprocNames unsets whatever published the ends.
//
// Only the dialect with an array has anything to unset, and it is asked as the
// same question that put the names there: where the ends are published in an
// array, the array is how a script reaches them, so an array outliving the
// ends would name two descriptors that are gone.
//
// Unset rather than emptied, measured: `declare -p CP` after the reaping
// answers `CP: not found` rather than an empty array, and `${CP+set}` is
// empty. The redirection then has an empty word for a target, which is the
// `ambiguous redirect` a script sees.
func (r *Runner) forgetCoprocNames() {
	if r.coproc == nil || r.coproc.name == "" {
		return
	}
	if r.sem().CoprocEndsInAnArray != Yes {
		return
	}
	r.unsetName(r.coproc.name)
	r.unsetName(r.coproc.name + "_PID")
}

// coprocEnds is the pair of descriptors a running coprocess is reached by,
// with the job that is running behind them.
//
// The job is kept because one spelling asks whether the coprocess is still
// alive rather than whether one was ever started: ksh93's `|&` refuses a
// second while the first runs and accepts one after it has ended.
type coprocEnds struct {
	read, write int
	job         *Job

	// name is what the array publishing the ends is called, and is empty for
	// the two spellings that publish nothing — zsh's `coproc`, which has no
	// name for a coprocess, and ksh93's `|&`, which has no word in front of
	// it to carry one. It is recorded when the array is written rather than
	// when the coprocess starts, so the name here is exactly the name that
	// was published and never one this shell declined to publish under.
	name string

	// owner is the runner that started it, and is what makes retireCoproc
	// safe to call from every statement. A subshell is a shallow copy of its
	// parent, so it holds this same pointer and would otherwise retire the
	// *parent's* coprocess — unsetting an array and dropping descriptors in a
	// runner that is about to be discarded, while the shell that owns them
	// still holds both. A coprocess started inside a subshell is owned by
	// that subshell and retires there, which is the same rule read the other
	// way.
	owner *Runner

	// retired records that the ends have already been let go of, so a
	// coprocess is retired once rather than at the top of every statement
	// after it ended. It also keeps the write end's disposal from being
	// undone by the read end's, which arrives later in one dialect.
	retired bool

	// readEnded records that a read of the near end has reached end-of-file,
	// which is when both dialects with the coprocess letters let go of the
	// whole coprocess. See coprocReadEnded, and note that this is not the
	// reaping: it happens with the coprocess still unreaped and it happens
	// whatever ReapedCoprocessEnds says.
	readEnded bool
}

// CoprocRead and CoprocWrite are the descriptors of the running coprocess, for
// a dialect builtin that speaks to one by a letter rather than through an
// array — `print -p` and `read -p`. The second result is false when no
// coprocess has been started, which is the refusal both letters measure.
//
// An end the reaping already took back answers false as well, and it is the
// *same* refusal rather than a second one: measured 2026-09-12, ksh93 answers
// `print: no query process` word for word whether no coprocess was ever
// started or the one that was has ended. The two ends are asked separately
// because that dialect lets go of them separately — see
// Semantics.ReapedCoprocessEnds.
func (r *Runner) CoprocRead() (int, bool) {
	if r.coproc == nil || r.coproc.read < 0 {
		return 0, false
	}
	return r.coproc.read, true
}

func (r *Runner) CoprocWrite() (int, bool) {
	if r.coproc == nil || r.coproc.write < 0 {
		return 0, false
	}
	return r.coproc.write, true
}

// coprocReader is the near end a `read -p` reads, watching for the end of it.
//
// It exists because the dialect that keeps a reaped coprocess's read end lets
// go of it on end-of-file rather than on the reaping, and nothing else in this
// shell is reading that pipe — so the read that found the end is the only
// thing that can say so. See Runner.coprocReadEnded.
type coprocReader struct {
	io.Reader
	r *Runner
}

func (c *coprocReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	if errors.Is(err, io.EOF) {
		c.r.coprocReadEnded()
	}
	return n, err
}

// coprocNoticedByAWrite delivers the reaping notice where this shell is about
// to *write* into the ends a coprocess published under a name.
//
// It is the fifth place the notice lands, and the only one that is not a
// child being reaped. The four in retireCoproc match every shape bash answers
// the same way twice; this one covers the shape bash answers by **winning a
// race**, which is the whole of #2582 and measured 2026-09-13 on bash 5.3.15:
//
//	coproc { echo hello; }
//	read -r out <&${COPROC[0]}
//	echo foo >&${COPROC[1]}      st=0, the shell carries on, 100 runs of 100
//
// The issue read that status as bash keeping a read end of the shell→
// coprocess pipe itself. It does not, and three measurements say so. `lsof`
// on the shell while the coprocess runs lists the two published descriptors
// and nothing else. A write of 200KB through the array — more than the pipe
// will hold, so it blocks until the child is gone — is SIGPIPE at 141. And a
// *duplicate* taken while the coprocess ran, written after a second read has
// proved the child closed its output, is 141 three times in three. The pipe
// has no reader once the child is gone; the small write above succeeds
// because the child has not finished dying yet.
//
// That race has both edges. With a hundred `:` between the read and the
// write, one run in twenty is 141 in bash itself; with five hundred, the
// notice has landed and it is `${CP[1]}: ambiguous redirect` at 1 (#2468).
// So bash has exactly two *deterministic* answers for a write aimed at a
// coprocess that has ended, and neither is a death: the ambiguous redirect
// where the array published the end, and `Bad file descriptor` where a script
// saved the number out of the array first.
//
// A coprocess here is a goroutine rather than a child, so its ends close the
// instant its body returns and this shell is never anywhere but the middle of
// that race — 141 every time, where bash is 141 almost never. Reproducing
// bash's window would be reproducing a race; taking the notice we already
// hold is deterministic and lands on the answer bash gives whenever it is
// asked twice.
//
// **A broken pipe is still a death**, and that is the control the fix must
// keep: `exec 3> >(exit 0); sleep 0.3; echo foo >&3` is 141 in bash, bash as
// `sh`, bash 3.2, zsh and here, and a duplicate of a coprocess's write end
// taken before the reaping is 141 after it — 30 runs of 30 in bash and here
// alike. Nothing about SIGPIPE changes; what changes is that the shell stops
// arriving at a broken pipe through a name it published itself.
//
// Three conditions, and each is measured rather than convenient:
//
//   - **Only a write.** `read -r a <&${CP[0]}` on a coprocess that has ended
//     still answers the line it wrote, with the array still at 2. The read
//     end has something in it; the write end has nobody at the other side.
//   - **Only onto a stream the command itself writes.** `exec 3>&${CP[1]}`
//     parks a *copy* on a number of the script's own, and that one bash
//     answers deterministically: the duplicate outlives the reaping and still
//     ends the shell on SIGPIPE when it is written, 30 runs of 30 in bash and
//     here alike. A notice delivered at the copy would turn a measured death
//     into a refusal, and would make it depend on whether a goroutine had got
//     round to finishing. So `{v}>&…` and any number above two are left
//     alone, and only 0, 1 and 2 — what the command in front of the
//     redirection goes on to write — deliver it.
//   - **Only through the published name.** `coproc CP { echo hi; }; echo x
//     >&2; echo "n=${#CP[@]}"` is `n=2` in bash, 30 runs of 30 — so an
//     ordinary `>&2` after a coprocess has ended does not deliver the notice,
//     and a rule keyed on the operator alone would answer 0 there.
//   - **Only where the ends were published under a name at all.** zsh has no
//     array and reaches its coprocess by a letter, and a `print -p` to one
//     that has ended is a SIGPIPE death at 141 — measured, and the opposite
//     answer. c.name is empty for that spelling and for ksh93's `|&`, so
//     neither is touched.
//
// The name is looked for in the redirection's text rather than in a parsed
// word, because a target is expanded from its text and there is no parsed
// form to ask. That is a guard and not a reading: its whole job is to keep
// `>&2` from delivering the notice, and a word that mentions the name and
// does not expand to the ends costs nothing but an earlier notice.
func (r *Runner) coprocNoticedByAWrite(rd *syntax.Redirect, fd int, fdVar string) {
	c := r.coproc
	// A coprocess whose ends were never published under a name is reached by
	// a letter rather than by a redirection, and the shells with the letters
	// answer this the other way.
	if c == nil || c.owner != r || c.retired || c.name == "" {
		return
	}
	if rd.Op != syntax.TokGreatAmp {
		return
	}
	// A name the shell picks is a number of the script's own, and the loop
	// has not picked it yet — fd is still the default 1 for `{v}>&…`, which
	// is why the name is asked about rather than only the number.
	if fdVar != "" || fd > 2 {
		return
	}
	if !strings.Contains(rd.Text, c.name) {
		return
	}
	// Still running is not ended: retireCoproc asks Job.Finished and does
	// nothing for a coprocess that is still there, which is what keeps
	// `coproc cat; echo hi >&"${COPROC[1]}"` working.
	r.retireCoproc()
}

// coprocRedirectWord is the word `>&` and `<&` take to mean the running
// coprocess in the two dialects that have the facility. One letter, and it is
// the same letter the builtins spell — see
// Semantics.CoprocessNamedByARedirection.
const coprocRedirectWord = "p"

// coprocRedirectEnd resolves that word to the number of the end the operator
// aims at: the end this shell *writes* for `>&p` and the end it *reads* for
// `<&p`.
//
// Which end goes with which operator is the facility's definition rather than
// a measurement to be split two ways — a coprocess is a pair of pipes and a
// redirection can only mean the one that runs the right way — and it is the
// same pairing `print -p` and `read -p` already use.
//
// The second result is false where the word is not the facility's, and where
// it is and no coprocess is running; the caller tells the two apart by asking
// coprocNamesARedirectionTarget first. A coprocess whose end the reaping has
// already taken back answers false as well, which is the same refusal a
// script gets before any coprocess was started — the two ends are asked
// separately because ksh93 lets go of them separately.
func (r *Runner) coprocRedirectEnd(op syntax.Kind) (int, bool) {
	switch op {
	case syntax.TokGreatAmp:
		return r.CoprocWrite()
	case syntax.TokLessAmp:
		return r.CoprocRead()
	}
	return 0, false
}

// coprocNamesARedirectionTarget answers whether this dialect reads `p` after
// `>&` or `<&` as the coprocess at all. False everywhere else, which is what
// leaves the word to the ordinary readings — a file in the dialect whose bare
// `>&word` is the csh spelling, and a refused target in the rest.
func (r *Runner) coprocNamesARedirectionTarget(op syntax.Kind, target string) bool {
	if target != coprocRedirectWord ||
		r.sem().CoprocessNamedByARedirection == CoprocessIsNotARedirectionTarget {
		return false
	}
	return op == syntax.TokGreatAmp || op == syntax.TokLessAmp
}

// coprocEndHandedOver is what the dialect that *moves* the end does once the
// duplication has happened: the end is no longer the coprocess's, so the
// letter that reached it finds none.
//
// One end at a time, measured — `exec 3>&p` leaves a `read -p` answering and
// `exec 4<&p` leaves a `print -p` writing — which is the same separation
// ReapedCoprocessEnds already needs and the same pair of fields it edits.
//
// The table entry goes and the file does not get closed, exactly as
// forgetCoprocFd has it: the number the script named is the same open file,
// and closing it underneath would end the pipe the move was for.
func (r *Runner) coprocEndHandedOver(op syntax.Kind) {
	c := r.coproc
	if c == nil || c.owner != r ||
		r.sem().CoprocessNamedByARedirection != CoprocessRedirectionMovesTheEnd {
		return
	}
	switch op {
	case syntax.TokGreatAmp:
		r.forgetCoprocFd(c.write)
		c.write = -1
	case syntax.TokLessAmp:
		r.forgetCoprocFd(c.read)
		c.read = -1
	}
}
