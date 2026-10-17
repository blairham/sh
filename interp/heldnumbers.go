// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/blairham/sh/syntax"
)

// The numbers a real shell holds for its own plumbing, and why they are
// bookkeeping here rather than descriptors.
//
// A shell that runs a redirected group in its own process has to put the
// streams back afterwards, so it keeps a copy of each one it replaced; a
// shell that runs a pipeline keeps the reading end of each pipe. Both are
// descriptors in its table, on the lowest free numbers from its allocation
// base up, so the dialect that publishes a substitution from that base steps
// over them: `{ echo <(true) <(true); } </dev/null` is `/dev/fd/12
// /dev/fd/13` in zsh 5.9.2, where the same echo with no redirection is 11 and
// 12 (#6473). Semantics.OwnDescriptors holds the measurements.
//
// This shell needs neither descriptor. A redirection replaces a stream in
// the runner and putting it back is an assignment, and a pipeline's ends are
// already moved out of the region a number is published from (see
// raiseShellEnd). So what is kept is the *number*, on the runner that would
// have held it, and the allocators skip it as they skip a number the script
// opened — see Runner.holdsNumber, which is the one predicate all of them ask.

// heldNumber is one number the imitated shell holds for itself.
//
// keptByAFork is the one property the two kinds differ on. A fork drops a
// saved copy — `{ ( echo <(true) ); } </dev/null` is 11, not 12 — and keeps a
// pipe's reading end: `true | true | { ( echo <(true) ); }` is 12, the first
// pipe's end still held in the parenthesized fork.
type heldNumber struct {
	fd          int
	keptByAFork bool
}

// keptByAFork is what a clone inherits of held: the pipe ends and none of the
// saved copies. A fresh slice, so neither side's later appends reach the
// other.
func keptByAFork(held []heldNumber) []heldNumber {
	var out []heldNumber
	for _, h := range held {
		if h.keptByAFork {
			out = append(out, h)
		}
	}
	return out
}

// holdsNumber is whether fd is taken in the table this shell is imitating:
// by something the script opened, or by the shell's own plumbing.
func (r *Runner) holdsNumber(fd int) bool {
	if _, held := r.fds[fd]; held {
		return true
	}
	for _, h := range r.heldNumbers {
		if h.fd == fd {
			return true
		}
	}
	for o := r.bodyOutput; o != nil; o = o.outer {
		if (o.held != 0 && o.held == fd) || int(o.fd.Load()) == fd || o.holds(fd) {
			return true
		}
	}
	return false
}

// bodyOutput is what a command substitution's body holds in ksh93u+: the
// number entering it held, if it held one (see enterSubstBody), and the
// body's output — zero until something in the body needs that output to be a
// descriptor, and the number it was put on from then until the body ends.
//
// ksh93u+ runs a command substitution's body without a fork and catches what
// it writes without a descriptor — until something needs one. **What decides
// is the body's standard output being needed as a descriptor**, not the kind
// of command: an external command or a pipeline needs it, and so does a
// redirection that names standard output, while a builtin writing to it or to
// another stream does not, and a process substitution, which is a fork,
// does. Measured 2026-10-15, `/bin/ksh -c` (ksh93u+
// 2012-08-01), the `{v}` allocated after the shape named (#6558):
//
//	echo $(exec {v}</dev/null; echo $v)                  11   see enterSubstBody
//	echo $(/usr/bin/true; exec {v}</dev/null; echo $v)   12   external
//	echo $(command -p true; exec {v}</dev/null; echo $v) 11   builtin
//	echo $(nosuchcmd 2>/dev/null; exec {v}…)             12   looked for outside
//	echo $(echo x | read y; exec {v}</dev/null; echo $v) 12   a pipeline of builtins
//	echo $(echo hi >&2; exec {v}</dev/null; echo $v)     12   names standard output
//	echo $(print -u2 hi; exec {v}</dev/null; echo $v)    11   writes elsewhere, names nothing
//	echo $(true >/dev/null; exec {v}</dev/null; echo $v) 12
//	echo $(true 2>/dev/null; exec {v}</dev/null; echo $v) 11
//	echo $(echo hi 2>&1; exec {v}</dev/null; echo $v)    12   standard output as the source
//	echo $( (/usr/bin/true); exec {v}</dev/null; …)      12   inside parentheses too
//	echo $(exec {v}</dev/null; /usr/bin/true; …{w}…)     11 13
//	echo $(: <(/bin/sh -c :); sleep 1; exec {w}…)        12   a process substitution
//	echo $(cat <(exec {v}</dev/null; echo $v))           12   whose body holds both
//
// It is taken once and kept: a second external command adds nothing. A body
// nested in this one sees it and has its own besides — `echo $(/usr/bin/true;
// echo $(exec {v}</dev/null; echo $v))` is 12 — which outer is for, while the
// inner body's own is gone with it: `echo $(echo $(/usr/bin/true); exec
// {v}</dev/null; echo $v)` is 11.
//
// Shared rather than copied, because the shape that needs it may run in a
// clone of the body — the parentheses row — and the number is the body's.
// A process substitution's body shares it too, as the last row says: unlike
// everything else a fork into that body drops (see substRunner), this is
// still held there, and the output is already a descriptor by then, so the
// body can add nothing to it that its parent would not.
type bodyOutput struct {
	held  int
	fd    atomic.Int32
	outer *bodyOutput
	// parens says this is a `( … )` ksh93 runs without a fork rather than a
	// command substitution's body: it holds a number the same way, and has
	// no output to make a descriptor of. See Runner.enterParentheses.
	parens bool
	// mu guards the two lists below, which an `exec` inside the body writes
	// and every allocation in it, and in what it runs concurrently, reads.
	mu sync.Mutex
	// decided are the streams an `exec` in this body has already reached,
	// and saved the numbers it holds for them until the body ends. See
	// redirHolds.saveForTheBody.
	decided, saved []int
	// jobHeld says a background job started in this body has held its one
	// number already. See Runner.forkInABody.
	jobHeld bool
}

// holds is whether this body's own `exec` saves hold fd.
func (o *bodyOutput) holds(fd int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Contains(o.saved, fd)
}

// enterSubstBody is what entering a command substitution's body holds, in
// the dialect that holds it: sub is the body's runner, cloned from r.
//
// ksh93u+ holds one number from its base for as long as a subshell it runs
// without a fork lasts, and a command substitution is one: `echo $(exec
// {v}</dev/null; echo $v)` is 11 there, from the base of 10, and so is every
// spelling of it — backquotes, an assignment's value, a function's body.
// One however deeply the substitutions nest, because the shell is already in
// such a subshell: `echo $(echo $(exec {v}</dev/null; echo $v))` is 11 too.
// From a script file every row is one higher than it is from `-c`, the file
// itself already being on 10. A process substitution's body is a fork and
// holds none of it — `echo <(echo $(exec {v}</dev/null; echo $v) >&2)` is
// 11, the outer `<( … )` adding nothing. Measured 2026-10-15 (#6558).
func (r *Runner) enterSubstBody(sub *Runner) {
	if r.sem().OwnDescriptors != LastPipelineElementsInputTakesANumber {
		return
	}
	o := &bodyOutput{outer: r.bodyOutput}
	if r.bodyOutput == nil {
		o.held = sub.lowestFreeNumber(-1)
	}
	sub.bodyOutput = o
}

// needsBodyOutput is something in a command substitution's body needing the
// body's output as a descriptor, which puts it on the lowest free number the
// first time. See bodyOutput.
//
// The parentheses have no output of their own to make one of, so it is the
// command substitution around them that does: `echo $( (/usr/bin/true);
// exec {v}</dev/null; echo $v)` is 12 in ksh93u+.
func (r *Runner) needsBodyOutput() {
	o := r.bodyOutput
	for o != nil && o.parens {
		o = o.outer
	}
	if o != nil && o.fd.Load() == 0 {
		o.fd.CompareAndSwap(0, int32(r.lowestFreeNumber(-1)))
	}
}

// forkInABody is what starting a fork inside a command substitution's body
// holds in ksh93u+, beyond what the body held already: the body's output as a
// descriptor, and one more number — for a pipeline element, in the element
// while it runs (element is its runner, nil for a background job); for a
// background job, in the body from then on, once however many jobs it
// starts. Measured 2026-10-16, `/bin/ksh -c` (ksh93u+ 2012-08-01), the `{v}`
// allocated after the shape named, from a body holding 10 (#6640):
//
//	echo $({ exec {v}…; echo $v; } | cat)               13   in the element
//	echo $({ …; } | cat; exec {w}…; echo $w)            13 12   gone with it
//	echo $(echo x | { …; } | cat)                       13
//	echo $(echo x | { …; })                             13   the last element: its input
//	echo $(true & wait; exec {v}…; echo $v)             13   and kept
//	echo $(/bin/sh -c : & wait; exec {v}…; echo $v)     13
//	echo $(true & true & wait; exec {v}…; echo $v)      13   once
//	echo $({ exec {v}…; echo $v; } & wait)              13   in the job as well
//	echo $(echo $(true & wait; exec {v}…; echo $v))     13   a nested body's own
//	( { …; } | cat ); :, ( true & wait; exec {v}… ); :  11   parentheses: nothing more
//
// Only directly in a command substitution's body: the parentheses have no
// output, and hold nothing more for a fork.
func (r *Runner) forkInABody(element *Runner) {
	o := r.bodyOutput
	if o == nil || o.parens || r.sem().OwnDescriptors != LastPipelineElementsInputTakesANumber {
		return
	}
	r.needsBodyOutput()
	if element != nil {
		element.holdNumber(true)
		return
	}
	o.mu.Lock()
	held := o.jobHeld
	o.jobHeld = true
	o.mu.Unlock()
	if held {
		return
	}
	n := r.lowestFreeNumber(-1)
	o.mu.Lock()
	o.saved = append(o.saved, n)
	o.mu.Unlock()
}

// enterParentheses is what a `( … )` ksh93 runs without a fork holds: sub is
// the parentheses' runner, cloned from r. Parentheses that are a pipeline
// element's or a background job's fork are that fork's last command, which
// holds nothing — see handForkTail.
//
// ksh93u+ forks the parentheses only where nothing follows them in the
// shell, and otherwise runs them in the shell as it runs a command
// substitution's body, holding one number from its base for as long as they
// last — the same one, so a body inside a body holds nothing more. Measured
// 2026-10-16, `/bin/ksh -c` (ksh93u+ 2012-08-01), the `{v}` allocated inside
// (#6638):
//
//	( exec {v}</dev/null; echo $v )                  10   the last command
//	( exec {v}</dev/null; echo $v ); :               11
//	( /usr/bin/true; exec {v}…; echo $v ); :         11   no output to need
//	( ( exec {v}…; echo $v ) ); :                    11   one hold, nested
//	echo $( ( exec {v}…; echo $v ) )                 11   and inside a body
//	( echo $(exec {v}…; echo $v) ); :                11
//	( echo $(/usr/bin/true; exec {v}…; echo $v) ); : 12   the body's output
//	( … ) && :, for i in 1; do ( … ); done, f        11
//	( … ) | cat; :, ( … ) & wait; :                  10   the fork itself
//	( ( … ) ) | cat; :                               10   the fork's last command
//	( ( … ); : ) | cat; :                            11
//	echo x | ( … ); :                                12   behind the last element's
//	( … ) 2>/dev/null; :                             12   behind the save of 2
//
// A process substitution's body is a fork: `cat <(exec {v}…; echo $v)` is 10.
func (r *Runner) enterParentheses(sub *Runner, c *syntax.Subshell) {
	if r.sem().OwnDescriptors != LastPipelineElementsInputTakesANumber || r.tailPutsNothingBack(c) {
		return
	}
	// Not a fork, so the saved copies the shell holds are held in here too,
	// the parentheses' own list's among them.
	sub.heldNumbers = slices.Clone(r.heldNumbers)
	o := &bodyOutput{outer: r.bodyOutput, parens: true}
	if r.bodyOutput == nil {
		o.held = sub.lowestFreeNumber(-1)
	}
	sub.bodyOutput = o
}

// handForkTail makes cmd the last thing a fork runs, in the dialect that
// saves nothing for its last command: ksh93u+ treats a pipeline element's or
// a background job's last command as the shell's own last. `{ { exec
// {v}</dev/null; echo $v; } 2>/dev/null; } | cat; :` is 10 there and the
// same with `:;` after the inner group is 11, and `&` answers the same pair.
// See tailPutsNothingBack.
func (r *Runner) handForkTail(cmd syntax.Command, pipe *syntax.Pipeline) {
	if r.sem().OwnDescriptors != LastPipelineElementsInputTakesANumber {
		return
	}
	r.tailCmd, r.tailPipe, r.tailPutsNothingBackOnly = cmd, pipe, true
}

// forkTailOf is the command and the pipeline a background job's statement
// ends on, as a fork that ran it would see them.
func forkTailOf(st *syntax.Stmt) (syntax.Command, *syntax.Pipeline) {
	body := *st
	body.Background, body.Coprocess, body.Disown = false, false, false
	list := []*syntax.Stmt{&body}
	return tailCommandOf(list), tailPipelineOf(list)
}

// substitutionPublishedAt is whether a live substitution of this shell's has
// published fd — a number really held in the shell being imitated, which an
// allocation from the base up has to step over. `: <(true) {v}</dev/null`
// leaves 12 in `v` in zsh 5.9.2, not 11.
//
// Not a number a fork into a substitution's body would have closed: those are
// the body's to re-use, and substFdView is where that is decided.
func (r *Runner) substitutionPublishedAt(fd int) bool {
	for _, p := range r.handedProcSubs() {
		if p.hold != nil && p.fd == fd && !slices.Contains(r.releasedSubstFds, fd) {
			return true
		}
	}
	return false
}

// holdNumber takes the number nextFreeFd would hand out next and holds it for
// the shell's own plumbing.
func (r *Runner) holdNumber(keptByAFork bool) int {
	fd := r.lowestFreeNumber(-1)
	r.heldNumbers = append(slices.Clip(r.heldNumbers), heldNumber{fd: fd, keptByAFork: keptByAFork})
	return fd
}

// letGoOfNumber gives a held number back. A fresh slice rather than a
// deletion in place, because a caller may hold the slice it is about to put
// back — see runPipeline.
func (r *Runner) letGoOfNumber(fd int) {
	i := slices.IndexFunc(r.heldNumbers, func(h heldNumber) bool { return h.fd == fd })
	if i < 0 {
		return
	}
	r.heldNumbers = slices.Concat(r.heldNumbers[:i], r.heldNumbers[i+1:])
}

// countsOwnDescriptors is whether this dialect's numbers step over all of
// zsh's plumbing — the saved streams, the pipes' reading ends and the order
// a list's substitutions are made in — which every caller asks first so the
// other dialects pay one comparison.
func (r *Runner) countsOwnDescriptors() bool {
	return r.sem().OwnDescriptors == SavedStreamsAndPipeReadEndsTakeNumbers
}

// savesRedirectedStreams is whether a redirection list run in the shell
// itself saves the streams it replaces on numbers a script can see: zsh's
// rule, bash's, which saves per redirection, or ksh93's, which saves nothing
// for the last command. See redirHolds.
func (r *Runner) savesRedirectedStreams() bool {
	o := r.sem().OwnDescriptors
	return o == SavedStreamsAndPipeReadEndsTakeNumbers || o == EachRedirectionSavesItsStreams ||
		o == LastPipelineElementsInputTakesANumber
}

// tailPutsNothingBack is whether c is the last thing this shell runs, in the
// dialect where that leaves a redirection list on it with nothing to save.
//
// ksh93u+ saves each stream a list replaces, like zsh, except on the last
// command of a `-c` string, where there is nothing left to put the stream
// back for. Measured 2026-10-16, `/bin/ksh -c` (ksh93u+ 2012-08-01), the
// `{v}` allocated inside the redirected group (#6637):
//
//	{ exec {v}</dev/null; echo $v; } </dev/null          10   the last command
//	{ exec {v}</dev/null; echo $v; } </dev/null; :       11   saved on 10
//	true && { … } </dev/null, if true; then { … } …; fi  10   still the last
//	{ … } </dev/null && :, ! { … } …, time { … } …       11
//	case a in a) { … } </dev/null;; esac                 10   but `;&` is 11
//	for i in 1; do { … } </dev/null; done                11   a loop's body never is
//	f(){ { … } </dev/null; }; f                          11   nor a function's
//	eval '{ … } </dev/null', command eval '…'            10   but an eval's text is
//	. /dev/stdin <<<'{ … } </dev/null'                   11   and a sourced file's is not
//	echo x | { … } 2>/dev/null                           11   the last element: its input
//	echo x | { … } 2>/dev/null; :                        12   is held on 10 (see
//	                                                          holdsTheLastElementsInput)
//	( { … } 2>/dev/null )                                10   parentheses run it through
//	( { … } 2>/dev/null; : )                             11
//	trap 'echo t' EXIT; { … } 2>/dev/null                10   a trap changes nothing
//
// From a script file nothing is last: `{ … } </dev/null` is 12 there, the
// file itself on 10. So this is tailexec.go's tail with three more places it
// reaches, none of them a place a shell becomes its last program: the last
// element of a pipeline, an `eval`'s text, a `( … )` — see
// tailPutsNothingBackOnly.
func (r *Runner) tailPutsNothingBack(c syntax.Command) bool {
	return r.putsNothingBackAtTheTail() && r.isTail(c)
}

// tailPipelinePutsNothingBack is tailPutsNothingBack for a pipeline of
// several, whose last element is the tail as far as that rule goes.
func (r *Runner) tailPipelinePutsNothingBack(p *syntax.Pipeline) bool {
	return r.putsNothingBackAtTheTail() && r.tailPipe == p && len(p.Cmds) > 1
}

// putsNothingBackAtTheTail is the half of tailPutsNothingBack that is not
// about which command: the dialect, and not inside a loop.
func (r *Runner) putsNothingBackAtTheTail() bool {
	return r.sem().OwnDescriptors == LastPipelineElementsInputTakesANumber && !r.tailInALoop
}

// handTailToPutNothingBack makes cmd the tail, and pipe the pipeline it
// stands in, for the save rule only — somewhere the shell runs a body
// straight through as its last and is still not the program it becomes —
// and answers what puts the old tail back. See tailPutsNothingBackOnly.
func (r *Runner) handTailToPutNothingBack(cmd syntax.Command, pipe *syntax.Pipeline) func() {
	oldCmd, oldPipe, oldOnly := r.tailCmd, r.tailPipe, r.tailPutsNothingBackOnly
	r.tailCmd, r.tailPipe, r.tailPutsNothingBackOnly = cmd, pipe, true
	return func() { r.tailCmd, r.tailPipe, r.tailPutsNothingBackOnly = oldCmd, oldPipe, oldOnly }
}

// holdsTheLastElementsInput is whether, in the dialect that holds a number for
// a pipeline's last element, this pipeline's last element holds one.
//
// ksh93u+ holds one number from its base while a pipeline's last element runs
// — however long the pipeline, and in a fork of the element as well — so a
// `{v}` allocated there is one higher: `echo x | { exec {v}</dev/null; echo
// $v; }` is 11, and `true | true | { … }` is 11 too. Only where the shell's
// own input was open, which is what says it is a copy of that input: after
// `exec 0<&-` it is 10. A process substitution's body holds none of it (see
// substRunner); a command substitution's body holds it and one number of its
// own (see enterSubstBody). Measured 2026-10-15 (#6503).
//
// One shape holds it once rather than twice: a last element that is a group
// holding nothing but another pipeline, which is the same pipeline as far as
// the input is concerned. `echo x | { echo y | { exec {v}</dev/null; echo
// $v; }; }` is 11, where a second command in the group, a redirection on it,
// a `!` or a `time` makes it 12.
func (r *Runner) holdsTheLastElementsInput(last syntax.Command) bool {
	return r.sem().OwnDescriptors == LastPipelineElementsInputTakesANumber &&
		r.streamOpen(0) && !aPipelineInBraces(last)
}

// aPipelineInBraces is whether c is `{ a | b; }`, through any number of such
// braces: one statement, nothing redirected or backgrounded, and that
// statement a pipeline of more than one element with no `!`.
func aPipelineInBraces(c syntax.Command) bool {
	for {
		g, ok := c.(*syntax.Group)
		if !ok || len(g.List) != 1 || len(g.Redirs) > 0 || g.List[0].Background {
			return false
		}
		switch x := g.List[0].Expr.(type) {
		case *syntax.Pipeline:
			if len(x.Cmds) == 1 && !x.Negated {
				c = x.Cmds[0]
				continue
			}
			return len(x.Cmds) > 1 && !x.Negated
		default:
			return false
		}
	}
}

// elementEntry is what entering a runner as a pipeline element or a
// background job owes the first redirection list it applies, which is the
// element's own.
//
// The pipe a pipeline element reads from is installed exactly as a `<`
// redirection would be, at the head of the element's own list. Where the
// element runs in the current shell, that means a save of standard input,
// taken while the pipe's end is still held — so the save lands one above it
// and the end's number then comes free: `echo x | { echo <(true) <(true)
// <(true); }` is `11 13 14` in zsh 5.9.2. Where it is a fork, nothing is
// saved, because a fork never puts anything back: `{ echo <(true); }
// </dev/null | cat` is 11, and so is the same group run with `&`.
type elementEntry struct {
	// pending says the next redirection list is the element's own.
	pending bool
	// forked says the element is a fork, whose own list saves nothing.
	forked bool
	// input is the pipe's reading end as the imitated shell numbers it, or
	// zero where the element reads no pipe. Never a real number below the
	// allocation base, so zero cannot be one.
	input int
	// substHeld is the number an element's first process substitution
	// holds until its words are expanded, once it is held, or zero. See
	// holdForTheFirstSubstitution.
	substHeld int
	// coprocess says the fork is a coprocess's, whose words hold its four
	// pipe ends instead of that one number: `coproc echo <(true) <(true)` is
	// `/dev/fd/15 /dev/fd/16` in zsh 5.9.2. See
	// CoprocEndsFirstAndFourthFromTheBase.
	coprocess bool
}

// redirHolds is what one redirection list holds while it applies: the saved
// copies it took, which streams it has already decided about, and whether it
// saves at all.
//
// Nil wherever the dialect counts nothing, so every method is safe on nil and
// the common path is one nil test.
type redirHolds struct {
	r *Runner
	// saving is false for a fork's own list and a command that is a process
	// of its own; `exec`'s list saves while it applies.
	saving bool
	// consuming is whether a substitution this list redirects from is taken
	// over by the redirection, so that its number comes free: every list
	// this shell runs in itself but `exec`'s.
	consuming bool
	// decided are the streams this list has already reached. A stream is
	// saved once, at the first redirection that names it, and only where it
	// was open *then*: `{ … } 3</dev/null 3</dev/null` saves nothing, the
	// second redirection finding 3 open only because the first opened it.
	decided []int
	// saved are the numbers this list holds, given back when the command
	// ends.
	saved []int
	// perRedirection says every redirection saves the stream it replaces,
	// where that stream is open at the moment it is reached, and no
	// duplication holds a copy of its source while it does: bash's rule.
	// `{ … } 2>/dev/null 2>/dev/null` saves twice there, and `{ … } 2>&1`
	// holds 10 where zsh holds 11 and leaves 10 free. See
	// EachRedirectionSavesItsStreams.
	perRedirection bool
	// holdsSources says a duplication's source sits on the lowest free
	// number while the stream it replaces is saved, and is let go of after:
	// zsh's rule. See save.
	holdsSources bool
	// savesMoveSources says a move's source is a stream the list replaces
	// too, saved as the destination is: ksh93u+'s rule. See moved.
	savesMoveSources bool
	// forTheBody says this is `exec`'s list inside a body the shell runs
	// without a fork, whose saves last as long as the body. See
	// saveForTheBody.
	forTheBody bool
	// joins says a stream redirected again in this list joins its targets
	// or sources, the dialect's multios, and reached counts how many
	// redirections of the list have named each stream so far. See join.
	joins   bool
	reached []streamReached
	// owed is the stream whose join of sources waits to hold one more
	// number after the next one anything in the list takes, or -1. See
	// join.
	owed int
	// joined are the numbers the list's joins hold, each with its stream,
	// given back once the list has applied or the join is ended.
	joined []streamHeld
}

// streamHeld is one number a join holds, and the stream joined.
type streamHeld struct{ fd, n int }

// streamReached is how many redirections of one list have named a stream.
type streamReached struct{ fd, n int }

// beginRedirHolds starts a redirection list's holds, settling first what the
// runner owes for having been entered as an element.
func (r *Runner) beginRedirHolds(owner redirOwner) *redirHolds {
	entry := r.entering
	r.entering = elementEntry{}
	if entry.substHeld != 0 {
		// The words are expanded and the list is reached. See
		// holdForTheFirstSubstitution.
		r.letGoOfNumber(entry.substHeld)
	}
	if !r.savesRedirectedStreams() {
		return nil
	}
	o := r.sem().OwnDescriptors
	// The parentheses are this shell's in the dialect that does not fork
	// them, and their own list saves as a group's does: `( … ) 2>/dev/null;
	// :` is 12 in ksh93u+. See enterParentheses.
	ownsTheParentheses := o == LastPipelineElementsInputTakesANumber && owner == redirOwnerASubshell
	inThisShell := (owner == redirOwnerThisShell || ownsTheParentheses) && r.redirectForBuiltin != "exec"
	// `exec`'s list saves what it replaces as any list in the shell does,
	// and gives the copies back once it has applied, which is the end of
	// the command it belongs to: `exec </dev/null {v}</dev/null` leaves 12 in
	// `v` in zsh 5.9.2 and 11 in bash 5.3.20, from bases of 11 and 10, and
	// `exec {w}</dev/null` after it is the base again in both. Measured
	// 2026-10-15 (#6529).
	execList := owner == redirOwnerThisShell && r.redirectForBuiltin == "exec"
	// The dialect that saves nothing for its last command saves nothing for
	// `exec` either, which is never put back — except inside a body it runs
	// without a fork, which puts it back when the body ends. See
	// tailPutsNothingBack and saveForTheBody.
	forTheBody := o == LastPipelineElementsInputTakesANumber && execList && r.bodyOutput != nil
	putsNothingBack := o == LastPipelineElementsInputTakesANumber && !forTheBody &&
		(execList || r.listOnTheTail && r.putsNothingBackAtTheTail())
	h := &redirHolds{
		r:                r,
		perRedirection:   o == EachRedirectionSavesItsStreams,
		holdsSources:     o == SavedStreamsAndPipeReadEndsTakeNumbers,
		savesMoveSources: o == LastPipelineElementsInputTakesANumber,
		forTheBody:       forTheBody,
		owed:             -1,
		joins:            r.countsOwnDescriptors() && r.sem().RedirectsUseEveryTarget == Yes,
		// A consumed substitution's number comes free in zsh only: bash
		// publishes the next one below it however the first was used —
		// `{ echo <(true) <(true); } < <(true)` is `62 61` in 5.3.20.
		consuming: inThisShell && r.countsOwnDescriptors(),
		// A `( … )` is a fork, and a command that becomes a process of its
		// own puts nothing back in this one; `exec`'s list saves while it
		// applies, though nothing is put back after.
		saving: (inThisShell || execList) && (!entry.pending || !entry.forked) && !putsNothingBack,
	}
	if entry.pending && entry.input != 0 {
		// The pipe is the list's first redirection: standard input is saved
		// with the end still held, then the end is let go of.
		h.save(0, false)
		r.letGoOfNumber(entry.input)
	}
	return h
}

// streamOpen is whether fd is open in this shell before a redirection
// replaces it — the condition for there being anything to save.
func (r *Runner) streamOpen(fd int) bool {
	switch fd {
	case 0:
		_, closed := r.Stdin.(closedFd)
		return !closed
	case 1:
		_, closed := r.Stdout.(closedFd)
		return !closed
	case 2:
		_, closed := r.Stderr.(closedFd)
		return !closed
	}
	_, open := r.fds[fd]
	return open
}

// save is one redirection of fd reached: the stream is saved on the next free
// number if this list has not decided about it yet and it is open.
//
// duplicating says the redirection copies another descriptor, whose copy
// sits on the lowest free number while the save is taken and is let go of
// after — `{ … } 2>&1` holds 12 and leaves 11 free, where `2>/dev/null`
// holds 11.
func (h *redirHolds) save(fd int, duplicating bool) {
	if h == nil {
		return
	}
	if h.forTheBody {
		h.saveForTheBody(fd, true)
		return
	}
	if !h.perRedirection {
		if slices.Contains(h.decided, fd) {
			return
		}
		h.decided = append(h.decided, fd)
	}
	if !h.saving || !h.r.streamOpen(fd) {
		return
	}
	r := h.r
	source := -1
	if duplicating && h.holdsSources {
		source = r.holdNumber(false)
		h.took(-1)
	}
	h.saved = append(h.saved, r.holdNumber(false))
	h.took(-1)
	if source >= 0 {
		r.letGoOfNumber(source)
	}
}

// closed is save for a close: the same, except that a close puts nothing on
// the number. See saveForTheBody.
func (h *redirHolds) closed(fd int) {
	if h != nil && h.forTheBody {
		if !h.perRedirection {
			h.saveForTheBody(fd, false)
		}
		return
	}
	h.save(fd, false)
}

// saveForTheBody is save for `exec` inside a body ksh93u+ runs without a fork
// — a command substitution's, or parentheses that are not the fork itself:
// the stream is saved for as long as the body lasts, so that it can be put
// back when it ends, and once however many `exec`s name it.
//
// Which streams is the part to read carefully. Measured 2026-10-16, `/bin/ksh
// -c` (ksh93u+ 2012-08-01), the `{v}` allocated after, from a body holding 10
// (#6639):
//
//	echo $(exec 2>/dev/null; exec {v}…; echo $v)             12
//	echo $(exec 0</dev/null; exec 2>/dev/null; exec {v}…)    13
//	echo $(exec 2>/dev/null; exec 2>/dev/null; exec {v}…)    12   once
//	echo $(exec 0<&-; exec {v}…)                             12   a close too
//	echo $(exec {v}…; exec 2>/dev/null; exec {w}…)           11 13
//	echo $(exec 3</dev/null; exec {v}…)                      12   3 was not open
//	echo $(exec 4</dev/null; exec {v}…) … 9                  11   nor were these
//	exec 4</dev/null; echo $(exec 4<&-; exec {v}…)           12   this one was
//	exec 3</dev/null; ( exec 4</dev/null; exec {v}…); :      12
//	( exec 4</dev/null; exec 4<&-; exec {v}…); :             11   decided at the first
//	( exec >/dev/null; exec {v}…; echo $v >&2 ); :           11   never standard output
//	( exec 2>/dev/null ); exec {v}…                          10   gone with the body
//
// So a stream is saved where it is open, or where it is the lowest number
// nothing is open on — the number an open would land on — and never standard
// output, which the body already holds as its own.
//
// opens says the redirection puts something on fd, which a close does not:
// `( exec 3<&-; exec {v}</dev/null; echo $v ); :` is 11, 3 being where an
// open would land and nothing landing there.
func (h *redirHolds) saveForTheBody(fd int, opens bool) {
	o := h.r.bodyOutput
	if o == nil || fd == 1 {
		return
	}
	r := h.r
	o.mu.Lock()
	decided := slices.Contains(o.decided, fd)
	if !decided {
		o.decided = append(o.decided, fd)
	}
	o.mu.Unlock()
	if decided || (!r.streamOpen(fd) && (!opens || fd != r.lowestUnopened())) {
		return
	}
	n := r.lowestFreeNumber(-1)
	o.mu.Lock()
	o.saved = append(o.saved, n)
	o.mu.Unlock()
}

// lowestUnopened is the lowest number nothing is open on, as this shell's
// table has it: the number a fresh open would land on.
func (r *Runner) lowestUnopened() int {
	n := 0
	for r.streamOpen(n) {
		n++
	}
	return n
}

// moved is a move reached whose source is from: `N<&M-`, `N>&M-` or
// `{v}<&M-` takes M away for the command and gives it back after, so in the
// dialect that saves it, M is saved as the stream the move replaces is.
// Measured 2026-10-16, `/bin/ksh -c` (ksh93u+ 2012-08-01), with 4 and 9 open
// on the null device and `{v}` after the move in the same list (#6747,
// #6814):
//
//	true 4<&9 {v}</dev/null                      11   4 saved; a copy is no move
//	true 4<&9- {v}</dev/null                     12   4 and 9 saved
//	true 1>&9- {v}</dev/null                     12   1 and 9
//	true 4<&9- {v}</dev/null, 4 not open         11   9 alone
//	true {v}<&9-                                 11   9 saved, then the name's pick
//	true {v}<&9-; true {w}</dev/null             11 10   given back after
//	exec {v}<&9-                                 10   `exec` saves nothing
func (h *redirHolds) moved(from int) {
	if h == nil || !h.savesMoveSources {
		return
	}
	h.save(from, false)
}

// join is a redirection of fd reached that joins the stream's earlier
// targets or sources, input saying which: zsh's multios, which holds numbers
// for the join from the redirection that starts it until the list has
// applied — only a `{v}` later in the same list can see them.
//
// Measured 2026-10-15 against zsh 5.9.2 run as `zsh -fc`, `v` from a base of
// 11 after each list; the pipe an element sits on is one of the stream's
// sources or targets, as the join itself has it (see pipemultio.go):
//
//	: </dev/null </dev/null {v}</dev/null                    14   save, 2
//	: </dev/null </dev/null </dev/null {v}</dev/null         16   save, 2, 1, owed
//	: 3</dev/null 3</dev/null {v}</dev/null                  13   2: 3 was not open
//	: </dev/null <<<x {v}</dev/null                          14   a body is a source
//	: </dev/null <&0 {v}</dev/null                           14   and so is a copy
//	: >/dev/null >/dev/null {v}</dev/null                    15   save, 3
//	: >/dev/null >/dev/null >/dev/null {v}</dev/null         16   save, 3, 1
//	: >/dev/null >&2 {v}</dev/null                           15
//	: 3>/dev/null 3>/dev/null {v}</dev/null                  14
//	f </dev/null </dev/null {v}</dev/null | cat              13   a fork's list saves nothing
//	f >/dev/null >/dev/null {v}</dev/null | cat              15   the pipe is the first target
//	echo x | : </dev/null {v}</dev/null                      14   the pipe is the first source
//	: </dev/null </dev/null {v}</dev/null {w}</dev/null      14 16
//	: </dev/null </dev/null 2>&1 {v}… {w}… {x}…              14 17 18
//	: </dev/null </dev/null 3</dev/null {v}… {w}…            14 16
//	: </dev/null </dev/null >/dev/null >/dev/null {v}…       19
//	{ exec {a}</dev/null; echo $a; } </dev/null </dev/null   12   only while the list applies
//	setopt nomultios; : </dev/null </dev/null {v}</dev/null  12   no join, nothing held
//
// So a join of targets holds three numbers and each further target one, and
// a join of sources holds two and each further source one — and owes one
// more, taken straight after the next number anything in the list takes,
// whether a save, a duplication's source, a `{v}` or another join's: the
// rows with `{w}` show the hole it leaves above `v`, and the `3<` row that
// a redirection taking no number from the base does not pay it.
func (h *redirHolds) join(fd int, input bool) {
	if h == nil || !h.joins {
		return
	}
	n := h.reach(fd)
	if n < 2 {
		return
	}
	count := 1
	if n == 2 {
		count = 2
		if !input {
			count = 3
		}
	}
	r := h.r
	for range count {
		h.joined = append(h.joined, streamHeld{fd: fd, n: r.holdNumber(false)})
		h.took(-1)
	}
	if n == 2 && input {
		h.owed = fd
	}
}

// substitutionANameTakes is the index of this command's live substitution
// that a `{name}` redirection's target is, where the name takes the
// substitution's own number rather than a new one, or -1.
//
// zsh 5.9.2 hands the name the substitution's number: `exec {v}< <(true)` is
// 11 and `exec {v}> >(cat)` is 12, the number the substitution was published
// on, and the next pick finds the other end's number free — `exec {v}>
// >(cat); exec {w}</dev/null {x}</dev/null` is `12 11 13` and the `<(…)`
// spelling `11 12 13`. Made before the list's other redirections, as every
// substitution of the list is (see makeFirst): `: {w}</dev/null {v}<
// <(true)` is `11 12` for `v` and `w`. `echo hi {v}>>(cat)` is 12 too.
// Without a name the number stays the command's, as it was. Measured
// 2026-10-15 (#6789).
//
// Only in the dialect that counts its own descriptors, which is the dialect
// it was measured in, and not for `{name}&>`.
func (r *Runner) substitutionANameTakes(name string, bothStreams bool) int {
	if bothStreams || !r.countsOwnDescriptors() {
		return -1
	}
	return r.consumedSubstitution(name)
}

// copiedFrom is a duplication reached whose source is fd, which ends a join
// of that stream: what it holds is given back, nothing more is owed for it,
// and the stream counts as reached once, by the join itself. zsh 5.9.2
// answers `12 13` for `: </dev/null </dev/null 3<&0 {v}</dev/null
// {w}</dev/null; echo $v $w` where the same list with no `3<&0` is `14 16`,
// and `12 14` for `: >/dev/null >/dev/null 2>&1 {v}… {w}…` where the
// targets' join alone is `15 16`. Then the stream can be joined again: `:
// </dev/null </dev/null {v}<&0 </dev/null {w}</dev/null` is `12 15`.
// Measured 2026-10-15 (#6529).
func (h *redirHolds) copiedFrom(fd int) {
	if h == nil || !h.joins {
		return
	}
	i := slices.IndexFunc(h.reached, func(s streamReached) bool { return s.fd == fd })
	if i < 0 || h.reached[i].n < 2 {
		return
	}
	h.reached[i].n = 1
	if h.owed == fd {
		h.owed = -1
	}
	kept := h.joined[:0]
	for _, j := range h.joined {
		if j.fd == fd {
			h.r.letGoOfNumber(j.n)
			continue
		}
		kept = append(kept, j)
	}
	h.joined = kept
}

// reach counts one more redirection of fd and answers how many there have
// been. A pipe the element sits on is reached before the list: see
// applyRedirList.
func (h *redirHolds) reach(fd int) int {
	if h == nil || !h.joins {
		return 0
	}
	for i := range h.reached {
		if h.reached[i].fd == fd {
			h.reached[i].n++
			return h.reached[i].n
		}
	}
	h.reached = append(h.reached, streamReached{fd: fd, n: 1})
	return 1
}

// took is the list having taken a number, which pays what a join of
// sources owes. taken is that number where nothing holds it yet — a `{v}`'s,
// chosen before it is opened — or -1.
func (h *redirHolds) took(taken int) {
	if h == nil || h.owed < 0 {
		return
	}
	owing := h.owed
	h.owed = -1
	r := h.r
	if taken >= 0 && !r.holdsNumber(taken) {
		r.heldNumbers = append(slices.Clip(r.heldNumbers), heldNumber{fd: taken})
		defer r.letGoOfNumber(taken)
	}
	h.joined = append(h.joined, streamHeld{fd: owing, n: r.holdNumber(false)})
}

// applied is the end of the list, and of what its joins hold.
func (h *redirHolds) applied() {
	if h == nil {
		return
	}
	for _, j := range h.joined {
		h.r.letGoOfNumber(j.n)
	}
	h.joined, h.owed = nil, -1
}

// Close gives the list's saved copies back, which is the end of the command
// they were saving for.
func (h *redirHolds) Close() error {
	for _, fd := range h.saved {
		h.r.letGoOfNumber(fd)
	}
	h.saved = nil
	return nil
}

// closers is the list's closers with the holds' own appended, where it holds
// anything.
func (h *redirHolds) closers(cs []io.Closer) []io.Closer {
	if h == nil || len(h.saved) == 0 {
		return cs
	}
	return append(cs, h)
}

// saveForAnOpen is save for a redirection that opens name onto fd — or onto
// both output streams, where fd is -1 — and the substitution it may consume.
//
// A substitution a redirection opens is a source like a duplication's: its
// number is held while the save is taken and comes free after. `{ echo
// <(true) <(true); } < <(true)` is `11 13` in zsh 5.9.2 and `3< <(true)` on
// the same group is `11 12`. It is held from before the list's first save,
// because the list's substitutions are all made first — see makeFirst — and
// a writing one is no exception: `exec 3>/dev/null; { … } 3> >(cat)` holds
// 11 only because that substitution is on 12, its reading end having taken
// 11 and given it back (see Runner.writingEndStepsOverTheReadingEnd).
func (h *redirHolds) saveForAnOpen(fd int, name string) {
	r := h.r
	i := -1
	if h.consuming {
		i = r.consumedSubstitution(name)
	}
	if fd < 0 {
		// Both streams onto one file, which is a source like a
		// duplication's: held while the saves are taken, let go of after.
		// See saveBothForAnOpen.
		h.saveBothForAnOpen()
	} else {
		h.save(fd, false)
	}
	if i >= 0 {
		r.releaseConsumedSubstitution(i)
	}
}

// saveBothForAnOpen is the saves and joins of `&>file`, which zsh 5.9.2
// takes stream by stream, standard output first — the save where the stream
// is open and undecided, then the join where it was named before — with the
// file it opened held on the lowest free number until the last save it takes,
// as `{ … } 2>&1` holds its source. Measured 2026-10-15 and 2026-10-16 under
// `-fc`, `v` and `w` allocated after (#6756):
//
//	: &>/dev/null {v}… {w}…              11 14   the file on 11, saves 12 13
//	: >/dev/null 2>&1 {v}… {w}…          12 14   (control)
//	: 2>/dev/null &>/dev/null {v}… {w}…  16 17   save 1, let go, join 2's three
//	: 1>/dev/null &>/dev/null {v}… {w}…  12 17   join 1's three, save 2, let go
//	: &>/dev/null >/dev/null {v}…        16      the join is the later `>`'s
//	: &>/dev/null &>/dev/null {v}…       19      no save, no file held: two joins
//	: >/dev/null 2>/dev/null &>… {v}…    19
func (h *redirHolds) saveBothForAnOpen() {
	if h == nil {
		return
	}
	r := h.r
	saves := func(fd int) bool {
		return h.saving && !slices.Contains(h.decided, fd) && r.streamOpen(fd)
	}
	last := -1
	for _, fd := range []int{1, 2} {
		if saves(fd) {
			last = fd
		}
	}
	source := -1
	if h.holdsSources && last >= 0 {
		source = r.holdNumber(false)
		h.took(-1)
	}
	for _, fd := range []int{1, 2} {
		h.save(fd, false)
		if fd == last && source >= 0 {
			r.letGoOfNumber(source)
		}
		h.join(fd, false)
	}
}

// consumedSubstitution is the index in this command's substitutions of the
// live one name is the path of, or -1.
func (r *Runner) consumedSubstitution(name string) int {
	return slices.IndexFunc(r.procSubs, func(p procSubPipe) bool {
		return p.hold != nil && p.fd > 0 && p.path == name
	})
}

// releaseConsumedSubstitution gives a consumed substitution's number back.
//
// Only the number. The pipe stays parked where it is until removeProcSubs,
// which is its lifetime and is what keeps a `>(cmd)`'s body reading — this
// shell's own open of the path holds the other reference. What changes is that
// no command is handed it any more, since a real shell has moved it onto the
// stream and closed it, and that the number may be published again: where the
// pipe really sits on it, it is registered as a private copy, which is exactly
// the case parkDescriptor publishes over by parking elsewhere. A number
// borrowed inside a substitution's body has no descriptor on it, so there is
// nothing to register — dropping it from the published list is the release.
func (r *Runner) releaseConsumedSubstitution(i int) {
	p := &r.procSubs[i]
	if int(p.hold.Fd()) == p.fd {
		holdPrivateCopy(p.fd)
		p.consumed = p.fd
	}
	p.fd = -1
}

// enterAsAnElement gives a forked pipeline element what a fork of the shell
// holds at that point: what it held before the pipeline, the reading ends of
// the pipes made so far — its own input among them — and the entry that
// installs that input.
//
// A `( … )` standing as an element holds none of the pipeline's ends: `true |
// true | ( echo <(true) )` is 11 in zsh 5.9.2, where `true | true | { echo
// <(true); } | cat` is 12.
func (sub *Runner) enterAsAnElement(before []heldNumber, reads []int, i int, cmd syntax.Command) {
	sub.heldNumbers = keptByAFork(before)
	if _, paren := cmd.(*syntax.Subshell); paren {
		return
	}
	for _, fd := range reads[:i] {
		sub.heldNumbers = append(sub.heldNumbers, heldNumber{fd: fd, keptByAFork: true})
	}
	sub.entering = elementEntry{pending: true, forked: true}
	if i > 0 {
		sub.entering.input = reads[i-1]
	}
}

// holdForTheFirstSubstitution is a process substitution about to be made in
// the words of a forked pipeline element before the element's own
// redirection list: the first one holds a number until the words are all
// expanded.
//
// zsh 5.9.2 holds one more number there, from the first substitution's pipe
// on, and only there: `echo <(true) <(true) | cat` is `11 13`, where the same
// command alone, as the last element, run with `&`, in a group or in
// parentheses is `11 12`. It is taken after the first end of that pipe —
// `echo >(true) >(true) | cat` is `13 14`, the reading end on 11 and the
// writing end the substitution publishes stepping over both — and given back
// before the command's redirections: `f(){ exec {w}</dev/null; echo $w
// "$@"; }; f <(true) <(true) | cat` is `12 11 13`, and with `{v}</dev/null`
// on `f` it is `v` that is 12. A command substitution in a later word holds
// it, being a fork of the element — `echo <(true) $(exec {v}</dev/null; echo
// $v) | cat` is `11 13` — and makes nothing: `echo $(true) <(true) <(true) |
// cat` is `11 13` too, and so is a file substitution before the pipes.
// Measured 2026-10-15 (#6530).
//
// So the number is the one after the lowest free one, whichever end of the
// pipe then takes the lowest.
//
// What decides is whether the words are expanded before the element's own
// list, which settles the entry, and not the command's kind, so nothing here
// tests it: `for i in <(true) <(true); do echo $i; done | cat` is `11 12`,
// as are a `case` word and an `if` or `while` condition, where `repeat 1
// echo <(true) <(true) | cat` is `11 13`.
func (r *Runner) holdForTheFirstSubstitution() {
	e := &r.entering
	if !e.pending || e.coprocess || e.substHeld != 0 || !r.countsOwnDescriptors() {
		return
	}
	end := r.holdNumber(true)
	e.substHeld = r.holdNumber(true)
	r.letGoOfNumber(end)
}

// madeFirstSubst is one process substitution of a redirection list, made
// before any redirection of the list was applied.
type madeFirstSubst struct {
	pos  syntax.Pos
	path string
	ok   bool
}

// makeFirst makes every process substitution written in the list's words
// before the first of its redirections is applied, and answers what puts the
// runner back once the list is done.
//
// zsh 5.9.2 does, and it is the order the numbers come out in: the list's
// substitutions are on their numbers before any stream is saved, so `{ echo
// <(true) <(true); } 2>/dev/null < <(true)` is `11 14` there — the
// substitution on 11, standard error saved on 12 and standard input on 13,
// then 11 given back as standard input takes the pipe over. Made where it is
// reached, it was `12 14`. The order is also visible without a number: `:
// </nonexistent > >(echo ran >&2)` prints `ran` in zsh, whose first
// redirection fails after the second's substitution was started, and in
// neither bash 5.3.20 nor ksh93u+. Only the substitutions: a command
// substitution in a later word is not run, `: </nonexistent >$(echo ran >&2)`
// printing nothing in all three. And after what entering as a pipeline
// element owes, which is the list's first redirection (see beginRedirHolds):
// `echo x | { … } 2>/dev/null < <(true)` holds 12 and 13 as the group does
// outside a pipeline. Measured 2026-10-15 (#6505).
//
// Nil-safe like every method here, so the dialects that count nothing make
// each substitution where its word is expanded, as before.
func (h *redirHolds) makeFirst(ctx context.Context, rs []*syntax.Redirect) func() {
	if h == nil || !h.r.countsOwnDescriptors() {
		return func() {}
	}
	r := h.r
	outer := r.madeFirst
	var made []madeFirstSubst
	for _, rd := range rs {
		if rd.Word == nil || rd.Heredoc != nil {
			continue
		}
		for _, s := range rd.Word.Spans {
			if s.Kind != syntax.ProcSubstIn && s.Kind != syntax.ProcSubstOut {
				continue
			}
			// Made as the target's own expansion would make it: a
			// redirection target's body traces nowhere. See redirectTarget.
			wasTarget := r.redirectTargetWord
			r.redirectTargetWord = true
			path, ok := r.procSub(ctx, s)
			r.redirectTargetWord = wasTarget
			made = append(made, madeFirstSubst{pos: s.Pos, path: path, ok: ok})
		}
	}
	r.madeFirst = made
	return func() { r.madeFirst = outer }
}

// takeMadeFirst is the path a process substitution expands to: the one
// makeFirst made for it, or one made now.
//
// Taken rather than read, so that a word expanded twice makes its second
// substitution where it always did. A substitution that failed when it was
// made fails its word again without being retried — its complaint is already
// written.
func (r *Runner) takeMadeFirst(s syntax.Span) (string, bool) {
	for i, m := range r.madeFirst {
		if m.pos == s.Pos {
			r.madeFirst = slices.Delete(slices.Clip(r.madeFirst), i, i+1)
			if !m.ok {
				r.expandErr = true
			}
			return m.path, m.ok
		}
	}
	return r.procSub(r.ctx, s)
}
