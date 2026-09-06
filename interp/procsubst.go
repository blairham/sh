// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/blairham/sh/syntax"
)

// Process substitution: a command run with one end of a pipe, expanding to a
// path the other end can be opened by.
//
// It is the last of the core language — docs/spec/core.md has named it since
// before there was code to refuse it — and the reason it waited is that it is
// plumbing rather than grammar. A word that expands to text needs nothing from
// the process; a word that expands to a path is a promise that something can
// open that path, and keeping it reaches out of expansion entirely.
//
// The inner command runs concurrently, and that is not an optimization. Its
// output goes into a pipe whose buffer is finite, so running it to completion
// first would deadlock on anything longer than that buffer — which is most
// things worth substituting.
//
// A named pipe rather than /dev/fd, which was tried first and is worth
// recording. /dev/fd needs the descriptor to survive into the command that
// opens the path, and Go marks everything it opens close-on-exec — so it has
// to be cleared, and clearing it leaks the descriptor into *every* command the
// shell runs afterwards. That is not a tidiness problem: a later command
// holding the write end open means the substitution never sees end-of-file, so
//
//	echo x | tee >(tr a-z A-Z); sleep 0.4
//
// produced nothing at all, because `sleep` was holding the pipe. Clearing the
// flag only around the right fork is what a shell in C does; Go's os/exec
// takes the fork lock itself, so there is no window a caller can hold.
//
// A FIFO has none of that. It is a real path any process can open, inherits
// nothing, and leaks nothing — at the cost of a file to make and remove.

// procSub runs the inner command and returns the path naming its pipe.
func (r *Runner) procSub(ctx context.Context, kind syntax.SpanKind, src string) (string, bool) {
	f, perr := syntax.Parse(src, r.dialect())
	if perr != nil {
		r.diagf("%s\n", r.diag().ParseFailure(perr))
		r.expandErr = true
		return "", false
	}
	path, err := r.newFifo()
	if err != nil {
		r.diagf("%v\n", err)
		r.expandErr = true
		return "", false
	}
	// The shell's own end of the pipe is an open, and the gate is asked here
	// — on the calling goroutine, before anything is spawned — rather than
	// beside the blocking open below, so a denial never races anything and
	// aborts the substitution the way a refused redirect aborts its command:
	// before the inner command exists. The scaffolding around the open — the
	// temporary directory, the mkfifo, their removal — is deliberately not
	// gated; ActionOpen's comment in seams.go is the decision.
	action := r.act(Action{Kind: ActionOpen, Path: path, Write: kind != syntax.ProcSubstOut})
	if !r.allowed(ctx, action) {
		_ = os.Remove(path)
		r.expandErr = true
		return "", false
	}

	sub := r.clone()
	sub.inheritJobs(jobBoundarySubstitution)
	// The one boundary no shell's `trap` sees across: even the dialect that
	// keeps the parent's listing everywhere else lists nothing in
	// `<(trap)` — measured, `cat <(trap)` prints nothing in all three
	// shells that have the construct.
	sub.trapsModified()
	// A substitution runs beside the command that names it, so it shares the
	// caller's streams the way a background job does — and needs the same
	// guard for the same reason: an io.Writer carries no promise of being
	// safe to write from two places, and the shell is what created the
	// concurrency.
	//
	// Both sides, which is background()'s rule and is here for the reason it
	// gives: a lock one party takes and the other does not excludes nothing,
	// and the shell that named the substitution is the other party. It left
	// the shell writing raw — and a *child* of the shell too, because os/exec
	// copies into a writer that is not a file on a goroutine of its own, with
	// no share of this lock. `cat <(cmd)` raced on exactly that, and
	// bytes.Buffer grows before it reads, so a child that writes nothing at
	// all raced as well. That was #735.
	//
	// It costs nothing to wrap what is already a pipe: lockWriter leaves an
	// *os.File alone, so a shell whose streams are files hands its children
	// the descriptors, and one whose streams are not was giving them pipes
	// either way.
	// Both *streams*, not only the shared one, and that is the same argument
	// again: an embedder may hand one writer to Stdout and Stderr both, so
	// the substitution's diagnostic and the outer command's output are the
	// same object. Guarding only stderr left `cat <(cmd)` racing on the
	// stdout the shell had not wrapped — one mutex covers both streams for
	// exactly this reason; see lockedWriter.
	sub.Stderr = r.lockedStderr()
	r.Stderr, r.Stdout = sub.Stderr, r.lockedStdout()
	if kind == syntax.ProcSubstOut {
		sub.Stdout = r.Stdout
	}

	// `>(cmd)` reads the command's input out of the pipe, and the shell can
	// take that end without waiting for anybody — so it is taken here, on
	// this goroutine, and a failure is the shell's own and is reported like
	// one. `hold` is what makes waiting unnecessary; see openFifoReadEnd.
	var hold *os.File
	if kind == syntax.ProcSubstOut {
		var end *os.File
		var oerr error
		end, hold, oerr = openFifoReadEnd(path)
		if oerr != nil {
			_ = os.Remove(path)
			r.diagf("%v\n", oerr)
			r.expandErr = true
			return "", false
		}
		sub.Stdin = end
		r.spawn(func() {
			// Through the clone, which this goroutine owns: the record of
			// the open that the gate already allowed.
			sub.emit(ctx, Event{Kind: EventAccess, Action: action})
			if _, err := sub.Run(ctx, f); err != nil {
				sub.diagf("%v\n", err)
			}
		}, func() {
			// However the goroutine ended: this end closing is the
			// end-of-file the substituted command's reader is waiting for,
			// and skipping it would leave the command that named the path
			// waiting for one that is never coming.
			_ = end.Close()
		})
	} else {
		// `<(cmd)` writes cmd's output into the pipe, so this end is the
		// writer — and a writer has to wait for its reader, which is why
		// this half is on a goroutine and the other half is not. The wait
		// is bounded now rather than endless; openFifoWriteEnd is where
		// that is done and why.
		var end *os.File
		r.spawn(func() {
			var err error
			if end, err = openFifoWriteEnd(path); err != nil {
				// Nobody opened the other end — the command did not use the
				// path it was given, and the pipe went with it. There is
				// nothing to run and nothing to report: `echo <(true)`
				// prints a path and is not an error anywhere.
				return
			}
			sub.emit(ctx, Event{Kind: EventAccess, Action: action})
			sub.Stdout = end
			if _, err := sub.Run(ctx, f); err != nil {
				sub.diagf("%v\n", err)
			}
		}, func() {
			// The same close as the other direction, and the same reason:
			// it is the end-of-file the command that named the path is
			// reading until. Nil when nobody ever opened the other end, so
			// there was nothing to close.
			if end != nil {
				_ = end.Close()
			}
		})
	}

	r.procSubs = append(r.procSubs, procSubPipe{path: path, hold: hold})
	return path, true
}

// newFifo makes a named pipe in this shell's own directory.
//
// One directory per shell, made when the first substitution needs it, so a
// shell that never uses one leaves nothing behind. The names are numbered
// rather than random: they only have to be distinct within a directory nothing
// else writes to.
func (r *Runner) newFifo() (string, error) {
	dir, err := r.procSubDir()
	if err != nil {
		return "", err
	}
	r.procSubSeq++
	path := filepath.Join(dir, "sub"+strconv.Itoa(r.procSubSeq))
	if err := mkfifo(path); err != nil {
		return "", err
	}
	return path, nil
}

// procSubDirs is the directory each shell makes for its named pipes, and the
// lock that keeps two goroutines from making two.
//
// On the Runner rather than a package variable, because a Runner is a shell
// and two of them in one program each want their own — the same reason the
// working directory is a field.
type procSubDirs struct {
	once sync.Once
	dir  string
	err  error
}

func (r *Runner) procSubDir() (string, error) {
	if r.procSubHome == nil {
		r.procSubHome = &procSubDirs{}
	}
	// Read out here rather than inside the closure: this runs on the
	// goroutine that is expanding the word, which is where every other read
	// of the variable table happens, and once.Do would otherwise be the one
	// place a second goroutine reads r.Vars.
	home := r.tempHome()
	r.procSubHome.once.Do(func() {
		// An explicit parent, never MkdirTemp's empty one. Empty means
		// os.TempDir, which is os.Getenv("TMPDIR") wearing a different name
		// — the process's environment, read from inside the library, on the
		// live path of every substitution. It is the os.Getwd fallback the
		// glob path used to have, in a second place.
		//nolint:forbidigo // the parent is the Runner's, computed above; only MkdirTemp's empty-string form asks the process
		r.procSubHome.dir, r.procSubHome.err = os.MkdirTemp(home, "sh-procsub")
	})
	return r.procSubHome.dir, r.procSubHome.err
}

// tempHome is where this shell puts what it has to write to disk.
//
// TMPDIR through r.getVar, which is the Runner's own answer: its variables
// first and the environment its embedder handed in second. That is the same
// route `~` takes to HOME, and it is the only one that keeps two Runners in
// one program separable — an embedder that seeds Env decides where its
// shell's pipes go, and a script that assigns TMPDIR moves its own and
// nobody else's.
//
// No shell in the panel exposes this: measured on darwin, bash, zsh and
// ksh93 all expand `<(cmd)` to a /dev/fd path and none of them consults
// TMPDIR for it, while zsh's `=(cmd)` — the nearest construct that does
// write a file — reads TMPPREFIX and ignores TMPDIR too. So there is no
// behavior to match here and no axis to add. The named pipe is ours, forced
// by Go's close-on-exec (see the file comment), and where it lives is
// therefore our decision rather than a compatibility question. What settles
// it is the library rule: the answer belongs to the Runner.
//
// A relative TMPDIR is resolved against r.Dir rather than left for the
// operating system to resolve, because the directory the *process* happens
// to sit in is exactly the ambient state this is here to stop reading.
func (r *Runner) tempHome() string {
	dir, _ := r.getVar("TMPDIR")
	if dir == "" {
		// POSIX names /tmp as the directory that is there. A Runner built
		// with no Env at all — the zero value, which this package promises
		// is usable — still has to be able to make a pipe.
		return "/tmp"
	}
	if !filepath.IsAbs(dir) {
		return filepath.Join(r.workDir(), dir)
	}
	return dir
}

// procSubPipe is one substitution's named pipe: the path a command was given,
// and — for `>(cmd)` only — the descriptor holding that pipe open until the
// command is done with it. See openFifoReadEnd.
type procSubPipe struct {
	path string
	hold *os.File
}

// takeProcSubs hands over the pipes a command's substitutions made, and
// forgets them.
//
// Taken rather than read, because they belong to one command: the next one has
// its own, and a path left on the runner would be removed after some later
// command that never mentioned it.
func (r *Runner) takeProcSubs() []procSubPipe {
	pipes := r.procSubs
	r.procSubs = nil
	return pipes
}

// removeProcSubs takes away what a command's substitutions left behind.
//
// The pipe is removed as soon as the command that named it is done. Whatever
// is still reading or writing it holds an open file and does not care that the
// name is gone, which is the property that makes this safe to do early rather
// than at exit — a long session would otherwise fill its directory.
//
// And it is what ends the two waits a substitution can be in, which is why
// this runs even when the command never touched the path. The placeholder
// closing is the end-of-file a `>(cmd)` is waiting for; the name going away is
// the answer a `<(cmd)`'s writer is waiting for. Neither can outlive the
// command that named the path.
func removeProcSubs(pipes []procSubPipe) {
	for _, p := range pipes {
		if p.hold != nil {
			_ = p.hold.Close()
		}
		_ = os.Remove(p.path)
	}
}

// CleanUp removes what this shell made for itself.
//
// Only the directory the named pipes went in, at present. A caller that runs
// many scripts on one Runner should call it when finished; a binary that exits
// need not, since the directory is under this shell's temporary one — TMPDIR
// as the Runner reads it, which for a shell binary is the machine's. An
// embedder that points a Runner's TMPDIR somewhere of its own is the case
// where nothing else would ever remove it, which is the reason this is public.
func (r *Runner) CleanUp() {
	if r.procSubHome != nil && r.procSubHome.dir != "" {
		_ = os.RemoveAll(r.procSubHome.dir)
	}
}
