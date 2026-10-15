// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	// ident is what dir answered to a stat when the hold was made, and is
	// what dirNow compares a name against. A descriptor's identity — its
	// device and inode — cannot change while it is open, so asking the
	// descriptor again on every relative path bought nothing, and it was one
	// of the three system calls a relative file test made (#6423). Nil where
	// that stat failed; dirNow then asks as it did before.
	ident os.FileInfo
	// checked says a lookup from dir is permission-checked at every lookup,
	// as one from a process's working directory is, so StatAt from it may
	// stand in for a relative path. See opened.HoldChecked: false for a
	// Darwin hold that could only be opened to pass through.
	checked bool
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
	// copyFd is dir's number where it is registered as a private copy, and -1
	// where it is not; cleanup is what releases that registration and closes
	// dir for a shared hold nobody closes. See newHeldDirectory.
	copyFd  int
	cleanup runtime.Cleanup
	// byDescriptor is the path that leads to this directory through the
	// descriptor rather than through any name — opened.HeldName's answer,
	// verified — or "" where there is none. Asked only once the directory
	// has no name left, and computed once, since the descriptor's number is
	// fixed for its life. See dirNow.
	byDescriptorOnce sync.Once
	byDescriptor     string
}

// heldCopy is what a shared hold's cleanup needs: the file it closes and the
// number whose registration it releases first. Not the hold itself, which
// would keep the hold reachable for ever.
type heldCopy struct {
	dir *os.File
	fd  int
}

// newHeldDirectory makes dir the hold on the directory named, moved out of
// the numbers a substitution publishes.
//
// **A real shell's working directory is not a descriptor**, so nothing it does
// to its directory is in the table a substitution is numbered from. This one's
// is, and the open lands on the lowest free number — so the first external
// command a script ran left the hold on 3 or 4, and every substitution after
// it stepped over it: `/bin/echo x >/dev/null; echo <(true) <(true) <(true)`
// answered `3 5 6` in the ksh dialect where ksh93u+ answers `3 4 5`. And
// because the hold is opened lazily, by whichever element of a pipeline first
// asked where it was, a substitution in a sibling element raced it: `echo x |
// { echo <(true) <(true) <(true); } | { cat; }` answered `4 5 6`, `3 5 6` or
// `3 4 6` (#6454).
//
// So it is moved exactly as a body's private copies are: into
// privateCopyFloor's band, where a substitution would only reach it past
// twenty-odd live descriptors, and registered, so that one which does reaches
// it publishes the number rather than stepping over it. See dupFile.
//
// The registration has to end where the descriptor does, and a shared hold is
// never closed by anybody — it goes when the last copy holding it is
// collected. So the release and the close are one cleanup, in that order,
// which is the order releasePrivateCopy asks for; a hold closed on purpose
// stops the cleanup first, so the number is never released twice.
//
// A failure to move is not one: the hold stays where the open put it, which is
// where every hold was before this.
//
// The caller holds transientFds for reading, from the open that made dir to
// here, because until this returns the original is on a low number.
func newHeldDirectory(named string, dir *os.File, checked bool, floor int) *heldDirectory {
	h := &heldDirectory{named: named, dir: dir, checked: checked, copyFd: -1}
	// Read once, here, for dirNow: see ident. The copy below is the same
	// directory, so the identity read off the original is the copy's too.
	if ident, err := dir.Stat(); err == nil {
		h.ident = ident
	}
	raised, n, err := dupFile(dir, floor)
	if err != nil {
		return h
	}
	holdPrivateCopy(n)
	_ = dir.Close()
	h.dir, h.copyFd = raised, n
	h.cleanup = runtime.AddCleanup(h, func(c heldCopy) {
		opened.ForgetHeld(c.fd)
		releasePrivateCopy(c.fd)
		_ = c.dir.Close()
	}, heldCopy{dir: raised, fd: n})
	return h
}

// close lets go of the descriptor, releasing its registration first.
func (h *heldDirectory) close() {
	if h.copyFd >= 0 {
		h.cleanup.Stop()
		opened.ForgetHeld(h.copyFd)
		releasePrivateCopy(h.copyFd)
		h.copyFd = -1
	}
	_ = h.dir.Close()
}

// path is the path that reaches this directory through its descriptor, for a
// directory no name reaches any more, or "" where there is none.
//
// Only a hold moved into the private band has one. That is the hold with a
// cleanup, so its registration with opened ends exactly where its descriptor
// does; a hold left where the open put it is closed by os.File's own
// finalizer, which could not withdraw a registration first.
//
// Verified by asking it, because the answer is only worth handing out if it
// is this directory: on Linux it is /proc, which a container may not mount.
func (h *heldDirectory) path() string {
	if h.copyFd < 0 {
		return ""
	}
	h.byDescriptorOnce.Do(func() {
		p := opened.HeldName(h.copyFd)
		if p == "" {
			return
		}
		there, err := opened.AtHeld(p, os.Stat)
		if err != nil || h.ident == nil || !os.SameFile(there, h.ident) {
			opened.ForgetHeld(h.copyFd)
			return
		}
		h.byDescriptor = p
	})
	return h.byDescriptor
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
		//
		// And it is not only the leak. A hold taken at a subshell's `cd` is
		// what `( cd s; …s removed and made again…; [ -e f ] )` needs to
		// answer as zsh, bash, ksh93 and dash do (#6560), and it is an open
		// on every `$(cd dir && …)`: measured 2026-10-15 on Darwin, 2000 of
		// `x=$(cd /tmp && echo y)` went from 37 ms to 66 ms of CPU with the
		// hold taken there (paired median 1.73x, 15 of 15 slower), and the
		// bare open-dup-close-fstat it needs is ~7 µs against a ~19 µs
		// substitution. That is a hot idiom paying for a corner, so it is
		// left as filed rather than taken.
		return nil
	}
	// The open and the move out of the low numbers in one window — see
	// newHeldDirectory and transientFds.
	var held *heldDirectory
	err := withTransientFds(func() error {
		dir, checked, err := opened.HoldChecked(r.Dir)
		if err != nil {
			return err
		}
		held = newHeldDirectory(r.Dir, dir, checked, r.privateFdFloor())
		return nil
	})
	if err != nil {
		// Nothing is said about this. A directory the shell cannot open is
		// one it will hear about from the next thing it does there, in that
		// thing's own words, and a hold is an optimization of the truth
		// rather than a permission to be in a place.
		r.dropDirectoryHold()
		return nil
	}
	r.dropDirectoryHold()
	r.held = held
	return r.held
}

// dropDirectoryHold lets go of the hold this Runner has, closing the
// descriptor where this Runner is the only one that can reach it.
func (r *Runner) dropDirectoryHold() {
	if r.held == nil {
		return
	}
	if !r.held.shared.Load() {
		r.held.close()
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
	now, nameless := r.dirCalled()
	if nameless != nil && r.Gate == nil && r.Events == nil {
		// No name reaches the directory in hand, so the remembered one is
		// about to send the path somewhere else — to the directory made since
		// under the same name, or nowhere. The descriptor's own path reaches
		// it. Not under a gate or a sink: an action names an absolute path a
		// policy can read, and a descriptor's number is not one, so those
		// keep the name, as heldForRelativeProbe does. See
		// opened.HeldName.
		if p := nameless.path(); p != "" {
			return p
		}
	}
	return now
}

// dirCalled is dirNow's answer in names only: what this Runner's directory is
// called now, or the remembered name where nothing is. It is the right answer
// for a caller that *shows* the directory — `pwd -P`, the spelling one dialect
// hands an `exec`ed program as its name — and the wrong one for a caller about
// to hand a relative path to the system, which is dirNow's job.
//
// nameless is the hold where no name was found for it, and nil wherever the
// answer is a name that leads to the directory or there is no hold to ask.
func (r *Runner) dirCalled() (now string, nameless *heldDirectory) {
	held := r.holdDirectory()
	if held == nil {
		return r.workDir(), nil
	}
	here := held.ident
	if here == nil {
		var err error
		if here, err = held.dir.Stat(); err != nil {
			return r.workDir(), nil
		}
	}
	// **Not through the gate**, which is the one place in this package where
	// that is the right answer and it is worth saying why. Every stat in
	// fsgate.go is there because a probe is an oracle: `[ -f /etc/shadow ]`
	// learns something real about a path *a script chose*. These two ask
	// about the directory this shell is already in — the one a policy was
	// asked about at the `cd` that put it here — and the question is only
	// "am I still called that", whose answer the script cannot read except
	// through paths that do go through the gate. Putting them through it also
	// made every relative path in the shell emit a second ActionStat, which
	// TestEmptyFileOperandAsksTheFilesystemNothing reads as the shell asking
	// the filesystem something it was not asked to. See the entry for
	// interp.dirCalled in internal/boundary.
	if named, err := os.Stat(r.Dir); err == nil && os.SameFile(named, here) {
		return r.Dir, nil
	}
	now, ok := opened.Path(held.dir)
	if !ok {
		return r.workDir(), held
	}
	if there, err := os.Stat(now); err != nil || !os.SameFile(there, here) {
		return r.workDir(), held
	}
	return now, nil
}

// dirForReplacement is dirNow for a process replacement, which reads where to
// start and then lets go of everything the shell holds before the execve.
//
// Where dirNow's answer is the path through the held descriptor, that
// descriptor is what the replacement is about to move the process into, so it
// is taken out of the Runner's hands first — cleanUpAtEnd would otherwise
// close it between the two — and closed by release, which is only reached
// when the replacement failed and this process is a shell again.
func (r *Runner) dirForReplacement() (dir string, release func()) {
	dir = r.dirNow()
	h := r.held
	if h == nil || dir != h.path() {
		return dir, func() {}
	}
	r.held = nil
	return dir, func() {
		if !h.shared.Load() {
			h.close()
		}
	}
}

// heldForRelativeProbe is the hold a file test asks a relative operand of,
// or nil where the operand is to be asked by name.
//
// **Through the descriptor is one system call where by name is two**, and it
// is the answer the panel gives in the one place the two routes part. By name
// is dirNow and then the path: a stat of the remembered name, to check it
// still leads to the directory in hand, and a stat of the joined result. zsh
// makes one call, because its directory is the process's and a relative path
// is resolved from there by the kernel (#6494). fstatat on the held
// descriptor is that same question.
//
// Where the two routes part is a directory deleted and then **created again
// under the same name** while the shell is sitting in it. By name, dirNow
// finds the name leads to a different directory, the kernel has no name for
// the deleted one, and the answer falls back to the remembered name — the
// new directory. Measured 2026-10-15, a script that enters `a`, has `a`
// removed and recreated holding a file `f`, and asks:
//
//	                   [ -e f ]  [ -e . ]  [ -d . ]  ( [ -e f ] )
//	zsh 5.9.2          no        yes       yes       no
//	bash 5.3, 3.2      no        yes       yes       no
//	ksh93, dash        no        yes       yes       no
//	this shell, before yes       yes       yes       yes
//
// and with `a` removed and nothing in its place every shell in the panel
// still answers yes to `[ -e . ]` and `[ -d . ]` where this one answered no.
// The process's working directory is the deleted directory, so a relative
// path finds nothing new there; the descriptor is the same object, so asking
// it gives the panel's answer in every column. A rename, a directory reached
// through a symbolic link (the hold is the link's target, which the kernel
// resolves a relative `..` from as well), a directory made unsearchable or
// unreadable after the shell entered it, and a subshell that has not moved
// all answer the same both ways, measured alongside.
//
// nil — ask by name — wherever that would not hold:
//
//   - **A gate or an event sink.** A probe is an action and an action names an
//     absolute path; that path is dirNow's, so the route that builds it is
//     the route that is asked. The fast path is the no-policy one, as
//     fsgate.go's nil-nil path already is.
//   - **An absolute operand**, which has nothing to be relative to.
//   - **A hold checked only when it was opened**: Darwin's open to pass through
//     a directory is permission-checked at the open and never again, so a
//     directory made 0000 after the shell entered it would still answer
//     through it, where every shell in the panel but ksh93 is refused. A
//     readable hold is checked at every lookup, and is what Darwin holds
//     wherever the directory is readable; see opened.HoldChecked.
//   - **No hold**: no Dir, a directory that would not open, a platform with no
//     fstatat, or a subshell that has moved — holdDirectory's nil, which
//     dirNow reads the same way. That last one keeps this shell's old answer
//     for the deleted-and-recreated directory, since a copy that has done a
//     `cd` of its own holds nothing to ask.
func (r *Runner) heldForRelativeProbe(operand string) *heldDirectory {
	if !opened.StatAtSupported || r.Gate != nil || r.Events != nil || r.Dir == "" || filepath.IsAbs(operand) {
		return nil
	}
	if held := r.holdDirectory(); held != nil && held.checked {
		return held
	}
	return nil
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
		if here, err := r.stat(now); err != nil || !os.SameFile(here, there) {
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
	transientFds.RLock()
	defer transientFds.RUnlock()
	// Not checked: Within opens to pass through, which on Darwin is checked
	// only at the open, so a relative probe here goes by name. See
	// heldForRelativeProbe.
	r.held = newHeldDirectory(name, dir, false, r.privateFdFloor())
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
	// Through the gate, and before the move rather than after it. This is the
	// one route into a directory that does not go through `cd`'s own
	// `enterable` check — it resolves the operand from a descriptor, so a
	// policy that hid the path the script named would otherwise be walked
	// straight past by the retry. The place asked about is the one this is
	// about to reach, which is a different path from the one `enterable`
	// was given and so a question of its own rather than the same one twice.
	if r.Gate != nil || r.Events != nil {
		if r.probeDenied(r.act(Action{Kind: ActionStat, Path: filepath.Join(r.dirNow(), operand)})) {
			return "", false, false
		}
	}
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
