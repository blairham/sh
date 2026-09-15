// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"os/exec"
	"sync"
)

// The file-creation mask, held by the Runner rather than by the process.
//
// A mask is *process* state and this shell's subshells are not processes, so
// `umask 002` inside one reached the mask of the shell that started it and
// stayed there once the body ended. The direction is what made that a P1: a
// script doing its private work in `( umask 077; … )` is the harmless case,
// and any `( umask … )` at all before a later write silently *widened* the
// permissions of every file the script wrote afterwards (#2898).
//
//	umask 077
//	( umask 002 )
//	: > g           # -rw-rw-r-- there, -rw------- in all five columns
//
// #2898 reconstructed the boundary by hand — a save on the way in and a
// restore on the way out — and that mechanism could only ever reach the four
// bodies whose caller is *blocked* on them. A background job, a coprocess and
// a process substitution run beside their caller, so the one process mask is
// the shell's mask too for as long as such a body holds one, whatever happens
// at the end; and what the body found is stale by the time it could be handed
// back. Measured 2026-09-15 with the release wired into all seven,
// `cat <(umask 002) >/dev/null` left our zsh answering `077` for the rest of
// the script where real zsh answers `002` — a mask restored over a change the
// shell had made itself, three commands later. **A stale value put back over
// a live one is worse than the leak**, which is why #2949 is this file and
// not another caller of the old one.
//
// So the mask stops being held in the process at all. It is an ordinary field
// on the Runner, copied by clone() with everything else, and it reaches the
// kernel at the two points the kernel reads a mask:
//
//	an open   the mode this shell asks for is already masked, by
//	          Runner.CreationMode, so the kernel has nothing left to take
//	          away. See Runner.openGated.
//	a spawn   the child inherits the process's mask at the moment it is
//	          forked, so the mask is put on the process for the length of
//	          cmd.Start and taken straight off again. See startMasked.
//
// That dissolves the whole class rather than narrowing it. Two bodies running
// at once never see each other's mask, because neither of them is looking at
// the same place; a body's mask cannot outlive the body, because nothing
// outside the body ever reads it; and there is no window between a body's end
// and its caller's next command, because nothing is handed back.
//
// **The process's own mask is emptied once and stays empty**, which is the
// price of doing the masking by hand: a mask the kernel is still applying
// would be a second, invisible floor under every mode computed here, and
// `umask 002` inside a body could not widen past whatever the shell was
// started with. Runner.ensureUmask empties it, through the same hook, and
// keeps what it found as this shell's own starting mask.
//
// The consequence is the rule for anything in this tree that creates a file:
// **the kernel is no longer masking it, so it must be masked here.** Every
// open a script reaches goes through Runner.openGated, and the handful of
// builtins that create a file or a directory of their own call
// Runner.CreationMode. A site that forgets is not a subtle drift — it creates
// the mode it asked for, which for a directory is 0777.
//
// A Runner with no SetUmask hook is untouched by all of this: the process's
// mask is left exactly as it was, the kernel goes on applying it, and `umask`
// refuses for the reason it has refused since #117 — a mask this shell cannot
// change must not look as though it changed.

// spawnMask serializes the process's mask around a fork.
//
// The window is the process's and not a Runner's, so the lock is package-wide
// rather than per-shell: two background jobs with different masks are two
// goroutines reaching the same kernel field, and a mask left on by one while
// the other forks is the leak this file exists to remove, reappearing in the
// one place the mask still has to be real.
var spawnMask sync.Mutex

// ensureUmask takes this shell's starting mask off the process, once.
//
// Reading a mask means setting one — the system call offers no way to ask —
// so the read that learns the mask is also the write that empties it, which
// is exactly the state the rest of this file needs the process to be in.
//
// Called on the outermost Runner, from RunPart, before anything it might run
// can create a file; a clone inherits the answer along with every other
// field, and a shell started by runImageAsScript is handed it by hand. So the
// hook is reached once per process in a binary that is a shell, which is what
// makes "what was there before" a question with an answer.
func (r *Runner) ensureUmask() {
	if r.maskKnown || r.SetUmask == nil {
		return
	}
	old, err := r.SetUmask(0)
	if err != nil {
		// The hook refused, so this shell has no mask to offer and `umask`
		// says so. Nothing is masked by hand either, which leaves the kernel
		// doing what it was already doing.
		return
	}
	r.umask, r.maskKnown = old, true
}

// CreationMode is the permissions a file or a directory created by this shell
// is really made with: what was asked for, with the mask taken out.
//
// Exported because a dialect's builtins create files too, and the kernel is
// no longer taking the mask out for them — `zmodload zsh/files; mkdir d` asks
// for 0777 and would make a world-writable directory without this. A shell
// with no mask of its own answers with the mode unchanged, which is the
// kernel's own behavior for a process whose mask this shell never emptied.
//
// It is the permission bits only. A mask has nothing to say about the file
// type or about setuid, setgid and the sticky bit, so those cross untouched —
// `mkdir -m 1777` keeps its sticky bit and loses whatever of 0777 the mask
// denies, which is what the kernel does with the same two numbers.
func (r *Runner) CreationMode(perm fs.FileMode) fs.FileMode {
	if !r.maskKnown {
		return perm
	}
	return perm &^ fs.FileMode(r.umask&0o777)
}

// createMode is CreationMode for this package's own opens, which speak in the
// ints os.OpenFile's callers here already hold.
func (r *Runner) createMode(perm int) fs.FileMode {
	return r.CreationMode(fs.FileMode(perm))
}

// startMasked is cmd.Start with this shell's mask on the process for the
// length of the call.
//
// A child does not read the mask, it *inherits* one — copied out of the
// parent at the fork and applied by the kernel to every file the child
// creates for the rest of its life. So this is the one place the mask still
// has to be the process's, and the window is as short as a fork.
//
// Every command this shell starts comes through here, which is what makes the
// window short enough to be held under one lock: the alternative is a mask
// left on the process between a start and whatever runs next, which is the
// leak in a smaller room.
//
// A shell with no mask of its own does not touch the process at all, so the
// child inherits whatever the shell itself inherited.
func (r *Runner) startMasked(cmd *exec.Cmd) error {
	defer r.holdMaskForFork()()
	return cmd.Start()
}

// holdMaskForFork puts this shell's mask on the process and answers with what
// takes it off again.
//
// Split out of startMasked because `exec` needs the same thing around a
// process *replacement*, where there is no exec.Cmd and the call does not
// return when it works.
func (r *Runner) holdMaskForFork() func() {
	if !r.maskKnown || r.SetUmask == nil {
		// This shell has no mask of its own, so the process's is whatever it
		// always was and a child inherits that.
		return func() {}
	}
	spawnMask.Lock()
	if r.umask == 0 {
		// Nothing to put on — the process's mask is already the empty one
		// ensureUmask left, which is what a child of a shell at `umask 0`
		// should inherit. The lock is still taken, and that is the point: a
		// body beside this one may be inside its own window, and a fork that
		// skipped the lock would inherit *that* body's mask.
		return spawnMask.Unlock
	}
	if _, err := r.SetUmask(r.umask); err != nil {
		// A refusal has nowhere to go: the command is about to start either
		// way, and the hook that just declined is the one that would be asked
		// to undo it. The lock is released, because a failed set is not a
		// mask left on the process.
		spawnMask.Unlock()
		return func() {}
	}
	return func() {
		_, _ = r.SetUmask(0)
		spawnMask.Unlock()
	}
}
