// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"slices"

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
	return false
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
// the shell's own plumbing. ignoring is a number to count as free although it
// is held, or -1.
func (r *Runner) holdNumber(keptByAFork bool, ignoring int) int {
	fd := r.lowestFreeNumber(ignoring)
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

// countsOwnDescriptors is whether this dialect's numbers step over the
// shell's own plumbing at all, which every caller asks first so the other
// dialects pay one comparison.
func (r *Runner) countsOwnDescriptors() bool {
	return r.sem().OwnDescriptors == SavedStreamsAndPipeReadEndsTakeNumbers
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
}

// beginRedirHolds starts a redirection list's holds, settling first what the
// runner owes for having been entered as an element.
func (r *Runner) beginRedirHolds(owner redirOwner) *redirHolds {
	entry := r.entering
	r.entering = elementEntry{}
	if !r.countsOwnDescriptors() {
		return nil
	}
	inThisShell := owner == redirOwnerThisShell && r.redirectForBuiltin != "exec"
	h := &redirHolds{
		r:         r,
		consuming: inThisShell,
		// A `( … )` is a fork, `exec`'s list is never put back, and a command
		// that becomes a process of its own puts nothing back in this one.
		saving: inThisShell && !(entry.pending && entry.forked),
	}
	if entry.pending && entry.input != 0 {
		// The pipe is the list's first redirection: standard input is saved
		// with the end still held, then the end is let go of.
		h.save(0, false, -1)
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
// holds 11. ignoring is a number to count as free although it is held, or -1:
// the substitution an output redirection consumes, whose pipe a real shell
// makes only after the save. `exec 3>/dev/null; { … } 3> >(cat)` holds 11
// in zsh 5.9.2, as `3>/dev/null` would.
func (h *redirHolds) save(fd int, duplicating bool, ignoring int) {
	if h == nil || slices.Contains(h.decided, fd) {
		return
	}
	h.decided = append(h.decided, fd)
	if !h.saving || !h.r.streamOpen(fd) {
		return
	}
	r := h.r
	source := -1
	if duplicating {
		source = r.holdNumber(false, ignoring)
	}
	h.saved = append(h.saved, r.holdNumber(false, ignoring))
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
// A substitution a redirection reads from is a source like a duplication's:
// its number is held while the save is taken and comes free after. `{ echo
// <(true) <(true); } < <(true)` is `11 13` in zsh 5.9.2 and `3< <(true)` on
// the same group is `11 12`. One it writes into is made only after the save,
// so the save does not step over it: `exec 3>/dev/null; { … } 3> >(cat)`
// holds 11, exactly as `3>/dev/null` does.
func (h *redirHolds) saveForAnOpen(fd int, name string) {
	r := h.r
	i := -1
	if h.consuming {
		i = r.consumedSubstitution(name)
	}
	ignoring := -1
	if i >= 0 && r.procSubs[i].commandWrites {
		ignoring = r.procSubs[i].fd
	}
	if fd < 0 {
		h.save(1, false, ignoring)
		h.save(2, false, ignoring)
	} else {
		h.save(fd, false, ignoring)
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
