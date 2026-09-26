// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sync/atomic"

	"github.com/blairham/sh/internal/opened"
)

// A Runner's directory is a *name*, and a name is not the directory.
//
// `cd d; mv ../d ../e` leaves a shell whose `$PWD` no longer leads anywhere.
// Measured 2026-09-26, every real shell in the panel — zsh 5.9.2, bash 5.3,
// bash 3.2, ksh93, dash, BusyBox ash 1.37 — carries on regardless: an external
// command started afterwards runs in the renamed directory, a relative
// redirection writes into it, and `pwd -P` names it. This shell could do none
// of those. It joined the name it remembered onto the path and handed the
// result to the kernel, which answered that there was no such place, so
// `/bin/pwd` came back 126, `> rel.txt` came back 1 and `. ./src.sh` came back
// 127 (#4653).
//
// The reason those shells do not care is that the kernel is holding their
// directory for them. A process's working directory is a reference to the
// object, not to a path, so a rename moves the shell along with it and nothing
// in the shell has to notice. **This shell may not have one**: `interp` is a
// library, two Runners in one program must not fight over one process-wide
// directory, and `cd` therefore sets `r.Dir` and never calls `os.Chdir`. That
// rule is in AGENTS.md and it is not the thing standing in the way here — it
// is right, and it is right *inside the binary too*, because the binary runs
// several Runners at once in one process. A pipeline's stages, a command
// substitution and a process substitution are clones on goroutines, and
// `diff <(cd /; pwd) <(cd /usr; pwd)` is two of them with different
// directories running at the same time. A process cwd would make that a race
// whichever program held it.
//
// What the rule forbids is *process-wide* state. A descriptor is not that: it
// is per-Runner, exactly as the descriptor table a `exec 3>f` builds is, and
// it holds the object the same way the kernel's own cwd does. So a Runner
// keeps one open on the directory it is in, and asks it — rather than asking
// the process — what that directory is called now.
//
// Two things are deliberately *not* asked of it:
//
//   - `$PWD` and a bare `pwd`, which keep the name the directory was reached
//     by. Measured: after the rename above, zsh, bash, ksh93, dash and ash all
//     still print the old name from `pwd`, and only `pwd -P` moves. The hold
//     answers where the operating system is involved and the remembered name
//     answers where the script asked what it called this place.
//
//   - `cd`'s own arithmetic. `cd` builds its destination from `$PWD`, and the
//     rows that prove it are in builtin.go beside the fallback.
type heldDirectory struct {
	// named is r.Dir as it stood when this was opened, and is what says
	// whether the hold is still the right one to be holding. A Runner whose
	// Dir is assigned from outside — an embedder, or driver at startup — gets
	// a fresh hold the next time one is wanted rather than a stale one.
	named string
	// dir is the descriptor. It is what a rename moves and a name does not.
	dir *os.File
	// shared says a clone took this hold too, so the Runner that opened it may
	// no longer close it: a subshell is a copy running in the same process and
	// the copy is reading the same descriptor. Closing one out from under the
	// other would be an ordinary use-after-close, and a pipeline's stages run
	// at the same time, so this is read and written from more than one
	// goroutine. An unshared hold is closed at once when `cd` replaces it;
	// a shared one is dropped, and the descriptor goes back when the last
	// reference to the file is collected, which is the safety net os.File
	// carries for exactly this.
	shared atomic.Bool
}

// holdDirectory is the Runner's hold on its own directory, opened if there is
// not one already for the name the Runner is currently using.
//
// Nil is a perfectly good answer and every caller treats it as "ask the name":
// a Runner with no Dir at all, a directory that cannot be opened, a platform
// with no way to hold one. What nil must never mean is "the hold went stale
// and nobody noticed", which is what the named comparison is for.
func (r *Runner) holdDirectory() *heldDirectory {
	if r.Dir == "" {
		return nil
	}
	if r.held != nil {
		if r.held.named == r.Dir {
			return r.held
		}
		// The Runner has moved since this was taken, so it names somewhere
		// else and must not be read as this directory. Dropped rather than
		// kept: a copy that has done a `cd` of its own would otherwise answer
		// every question about where it is with the directory it was cloned
		// from.
		r.dropDirectoryHold()
	}
	if r.inSubshell {
		// **A copy never opens one of its own.** It inherits the descriptor
		// its parent had, which is what a fork inherits and is what makes
		// `mv ../d ../e; (cd .)` work; what it may not do is open a second
		// one, because a copy is dropped rather than closed and there is no
		// one place every kind of copy ends. Seven call sites clone a Runner
		// and five of them reach endSubshell, which is exactly the shape that
		// leaks a descriptor per command substitution and is caught two years
		// later. So the answer here is nil, every caller reads that as "ask
		// the name", and a subshell that has moved is back to what this shell
		// did before the hold existed — for the one combination of having
		// moved *and* being renamed out from under afterwards.
		return nil
	}
	dir, err := opened.Hold(r.Dir)
	if err != nil {
		// Nothing is said about this. A directory the shell cannot open is
		// one it will hear about from the next thing it does there, in that
		// thing's own words, and a hold is an optimization of the truth
		// rather than a permission to be in a place.
		r.dropDirectoryHold()
		return nil
	}
	r.dropDirectoryHold()
	r.held = &heldDirectory{named: r.Dir, dir: dir}
	return r.held
}

// dropDirectoryHold lets go of the hold this Runner has, closing the
// descriptor where this Runner is the only one that can reach it.
func (r *Runner) dropDirectoryHold() {
	if r.held == nil {
		return
	}
	if !r.held.shared.Load() {
		_ = r.held.dir.Close()
	}
	r.held = nil
}

// dirNow is workDir asked of the filesystem rather than of memory: where this
// Runner's directory is **called now**, for the callers that are about to hand
// a path to the operating system.
//
// The remembered name wins whenever it is still a name for the directory in
// hand, which is every ordinary case and, importantly, every symbolic-link
// case: after `cd sub/fake`, `r.Dir` is `sub/fake` and the hold is the
// directory the link points at, and those are the same directory — so the
// logical path survives and #4590's whole grid is untouched. The hold is read
// only where the remembered name has stopped naming it.
//
// "Has stopped naming it" is identity, not existence, and the difference is a
// measured row rather than a nicety. With `d` renamed to `e` and a *different*
// `d` then created, the reference shell's child processes still run in `e`:
// the name is back and it leads somewhere else, which is exactly the case a
// bare `os.Stat` for existence would get wrong.
//
// The kernel's answer is verified before it is used, because one of the two
// platforms will hand back a name that no longer names anything: a directory
// whose last link has gone still answers F_GETPATH on Darwin with the name it
// had, where Linux's /proc marks it deleted and this package reads that as
// nameless. Verifying makes the two agree, and it makes them agree on the
// answer the panel gives — after an rmdir every one of those six shells keeps
// printing the name it had.
func (r *Runner) dirNow() string {
	held := r.holdDirectory()
	if held == nil {
		return r.workDir()
	}
	here, err := held.dir.Stat()
	if err != nil {
		return r.workDir()
	}
	if named, err := os.Stat(r.Dir); err == nil && os.SameFile(named, here) {
		return r.Dir
	}
	now, ok := opened.Path(held.dir)
	if !ok {
		return r.workDir()
	}
	if there, err := os.Stat(now); err != nil || !os.SameFile(there, here) {
		return r.workDir()
	}
	return now
}

// enteredFromTheDirectoryHeld opens a directory named relative to the one this
// Runner is holding, and reports what the kernel calls the place it reached —
// the empty string where the kernel has no name for it.
//
// It is the *move* without the bookkeeping: nothing here becomes this Runner's
// hold, because whether the move counts is a question for the dialect and a
// `cd` that does not arrive must leave the shell holding where it was.
//
// The operand is used **as the script wrote it**, because that is what the
// reference does: the same word, resolved from the directory rather than from
// the name.
func (r *Runner) enteredFromTheDirectoryHeld(operand string) (arrived *os.File, name string, ok bool) {
	held := r.holdDirectory()
	if held == nil || operand == "" {
		return nil, "", false
	}
	dir, err := opened.Within(held.dir, operand)
	if err != nil {
		return nil, "", false
	}
	there, err := dir.Stat()
	if err != nil {
		_ = dir.Close()
		return nil, "", false
	}
	now, named := opened.Path(dir)
	if named {
		// Verified before it is used, because one of the two platforms hands
		// back a name that no longer names anything: a directory whose last
		// link has gone still answers F_GETPATH on Darwin with the name it
		// had, where Linux's /proc marks it deleted and internal/opened reads
		// that as nameless. Verifying makes the two platforms agree.
		if here, err := os.Stat(now); err != nil || !os.SameFile(here, there) {
			now = ""
		}
	} else {
		now = ""
	}
	return dir, now, true
}

// adoptTheDirectoryHeld makes a descriptor this Runner has just opened the
// hold it keeps, under the name the Runner has settled on.
func (r *Runner) adoptTheDirectoryHeld(dir *os.File, name string) {
	r.dropDirectoryHold()
	r.held = &heldDirectory{named: name, dir: dir}
}

// cdFromTheDirectoryHeld is `cd`'s last resort: the operand tried again from
// the directory this Runner is holding, for the shells that do that.
//
// It reports whether the question was put at all, because the question must
// not be put by a `cd` that could not act on any answer — `cd nosuchdir` in an
// ordinary directory reaches here with a hold and a relative operand and is
// refused by the kernel a second time, which is the same refusal and not a
// disagreement between shells. So the *move* is attempted first and the axis
// is read only once there is something for it to decide.
//
// That order is the opposite of the usual one and it is deliberate: reading
// the axis first would make a Runner with no answer refuse every failing `cd`
// rather than the one shape the shells part over.
func (r *Runner) cdFromTheDirectoryHeld(operand, built string) (arrived string, moved, asked bool) {
	dir, name, ok := r.enteredFromTheDirectoryHeld(operand)
	if !ok {
		return "", false, false
	}
	settled := ""
	switch r.cdDestinationIsNotThere() {
	case CdDestinationNotThereEntersAndTakesTheKernelsName:
		// An empty name is the move happening somewhere the kernel cannot
		// name, which on both platforms means the directory's last link has
		// gone. Measured: zsh, bash and ksh93 all answer `cd .` in a removed
		// directory with 0 and leave `$PWD` exactly as it was, which is the
		// built name — so the two answers meet there rather than parting.
		settled = name
		if settled == "" {
			settled = built
		}
	case CdDestinationNotThereEntersAndKeepsTheBuiltName:
		settled = built
	default:
		// Refused, or unanswered. The descriptor goes back and this Runner is
		// left holding exactly where it was: a `cd` that does not arrive must
		// move nothing, and the hold is how every relative path after it will
		// still find the renamed directory.
		_ = dir.Close()
		return "", false, true
	}
	r.adoptTheDirectoryHeld(dir, settled)
	return settled, true, true
}
