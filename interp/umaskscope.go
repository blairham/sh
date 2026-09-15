// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The file-creation mask a forked body gets, which is its own.
//
// A mask is *process* state and this shell's subshells are not processes, so
// `umask 002` inside one reached the mask of the shell that started it and
// stayed there once the body ended. The direction is what made it a P1: a
// script doing its private work in `( umask 077; … )` is the harmless case,
// and any `( umask … )` at all before a later write silently *widened* the
// permissions of every file the script wrote afterwards (#2898).
//
//	umask 077
//	( umask 002 )
//	: > g           # -rw-rw-r-- here, -rw------- in all five columns
//
// Every enclosure leaked and not only `( … )`, because every one of them is a
// fork in a real shell: `x=$( umask 002 )` and `umask 002 | cat` were
// measured leaking too, and a background job, a coprocess and a process
// substitution are the same shape.
//
// This is that boundary reconstructed by hand, which is what AGENTS.md says a
// goroutine-for-a-fork costs — the same job Runner.anchorForkedBody does for
// the process group and Runner.endSubshell does for the EXIT trap. One
// mechanism for the four bodies that take it rather than a save and a
// restore written out at each, for the reason endSubshell is one function: a
// second copy of a boundary's ending is where the next fix goes missing.
//
// **The line is drawn at whether the caller is blocked on the body**, and it
// is drawn there rather than at "every clone" because the other half cannot
// be done this way at all.
//
// A subshell, a command substitution, a pipeline element and a shebang-less
// script each hold their caller still: the runner that started the body
// cannot reach another command until the body has ended, so handing the mask
// back at that moment is exactly what the fork did and there is no window in
// between for anything to observe. Those four take it.
//
// A background job, a coprocess and a process substitution run *beside* their
// caller, which carries straight on. The process has one mask, so it is the
// shell's mask too for as long as such a body holds one, whatever this file
// does — and what the body found is stale by the time it could be handed
// back. Measured 2026-09-15 with the release wired into all seven:
// `cat <(umask 002) >/dev/null` left our zsh answering `077` for the rest of
// the script where real zsh answers `002`, because the substitution's body
// ended three commands later and put back a mask the shell had moved off.
// **A stale value restored over a live one is a worse failure than the leak**
// — it undoes a change the shell made itself — so those three keep the leak
// until the mask can be applied per open and per spawn rather than held in
// the process. See #2949.
//
// A pipeline inside a background job keeps the restore, because the question
// is asked of the *caller*: the job's body is blocked on its elements even
// though the shell is not. What the shell sees while the job runs is the
// job's gap and not the element's.
//
// `$(<file)` is the one clone that takes nothing. Nothing is executed there —
// no command, no builtin, no `umask` — so there is no mask for the body to
// move. See Runner.readFileSubst.

// forkMask gives a cloned runner the private mask a real shell's fork would
// have given it, and answers with what ends it.
//
// Called on the clone, before it runs, and its release must be called however
// the body ends — including when it ends by returning an error, which is why
// every caller either defers it or hangs it on the same release the body's
// descriptors go out with.
//
// It clears whatever the clone inherited from the body around it, which is
// what a nested body wants: in `( umask 002; ( umask 007 ); umask )` the
// third of those is 002, because the inner parentheses are a fork of their
// own and put back what *they* found rather than what the outer body found.
//
// Nothing is read or set here, and nothing is allocated. A body that never
// runs `umask` — which is nearly all of them, on a path as hot as a command
// substitution — costs this a field it was already copying and a branch that
// is not taken.
func (c *Runner) forkMask() func() {
	c.maskMoved = false
	return func() {
		if !c.maskMoved || c.SetUmask == nil {
			return
		}
		// Whatever the body left it at is not this shell's business; what
		// the body *found* is. A failure here has nowhere to go — the body
		// is over, its status is already decided, and the hook that just
		// refused is the same one that accepted the change being undone.
		_, _ = c.SetUmask(c.maskOuter)
	}
}

// setMask changes the process's file-creation mask, recording what a forked
// body found so that forkMask's release can put it back.
//
// Only the *first* change is recorded, which is what makes
// `( umask 002; umask 007 )` restore the mask the parentheses opened with
// rather than the 002 the body passed through on its way to 007.
//
// Reading does not come through here. Runner.currentUmask sets to read,
// because the system call offers no way to ask, but it sets the same value
// straight back — so a body that only *reports* the mask has moved nothing
// and leaves nothing to undo.
func (r *Runner) setMask(mask int) error {
	old, err := r.SetUmask(mask)
	if err != nil {
		return err
	}
	if !r.maskMoved {
		r.maskMoved, r.maskOuter = true, old
	}
	return nil
}
