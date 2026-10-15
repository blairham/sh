// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"slices"
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
		if o.held == fd || int(o.fd.Load()) == fd {
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
func (r *Runner) needsBodyOutput() {
	if o := r.bodyOutput; o != nil && o.fd.Load() == 0 {
		o.fd.CompareAndSwap(0, int32(r.lowestFreeNumber(-1)))
	}
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
// rule, or bash's, which saves per redirection. See redirHolds.
func (r *Runner) savesRedirectedStreams() bool {
	o := r.sem().OwnDescriptors
	return o == SavedStreamsAndPipeReadEndsTakeNumbers || o == EachRedirectionSavesItsStreams
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
}

// redirHolds is what one redirection list holds while it applies: the saved
// copies it took, which streams it has already decided about, and whether it
// saves at all.
//
// Nil wherever the dialect counts nothing, so every method is safe on nil and
// the common path is one nil test.
type redirHolds struct {
	r *Runner
	// saving is false for a list whose replacements are never put back —
	// `exec`'s — and for a fork's own list.
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
}

// beginRedirHolds starts a redirection list's holds, settling first what the
// runner owes for having been entered as an element.
func (r *Runner) beginRedirHolds(owner redirOwner) *redirHolds {
	entry := r.entering
	r.entering = elementEntry{}
	if !r.savesRedirectedStreams() {
		return nil
	}
	inThisShell := owner == redirOwnerThisShell && r.redirectForBuiltin != "exec"
	h := &redirHolds{
		r:              r,
		perRedirection: r.sem().OwnDescriptors == EachRedirectionSavesItsStreams,
		// A consumed substitution's number comes free in zsh only: bash
		// publishes the next one below it however the first was used —
		// `{ echo <(true) <(true); } < <(true)` is `62 61` in 5.3.20.
		consuming: inThisShell && r.countsOwnDescriptors(),
		// A `( … )` is a fork, `exec`'s list is never put back, and a command
		// that becomes a process of its own puts nothing back in this one.
		saving: inThisShell && (!entry.pending || !entry.forked),
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
	if duplicating && !h.perRedirection {
		source = r.holdNumber(false)
	}
	h.saved = append(h.saved, r.holdNumber(false))
	if source >= 0 {
		r.letGoOfNumber(source)
	}
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
		h.save(1, false)
		h.save(2, false)
	} else {
		h.save(fd, false)
	}
	if i >= 0 {
		r.releaseConsumedSubstitution(i)
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
