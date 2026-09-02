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

	sub := r.clone()
	// A substitution runs beside the command that names it, so it shares the
	// caller's streams the way a background job does — and needs the same
	// guard for the same reason: an io.Writer carries no promise of being
	// safe to write from two places, and the shell is what created the
	// concurrency.
	sub.Stderr = r.lockedStderr()
	if kind == syntax.ProcSubstOut {
		sub.Stdout = r.lockedStdout()
	}

	go func() {
		// Opening a FIFO blocks until the other end is opened, which is why
		// this is here and not above: the command that names the path has not
		// started yet, and the shell must not wait for it to.
		//
		// `<(cmd)` writes cmd's output into the pipe, so this end is the
		// writer; `>(cmd)` reads the command's input out of it.
		var end *os.File
		var err error
		if kind == syntax.ProcSubstOut {
			end, err = os.OpenFile(path, os.O_RDONLY, 0)
		} else {
			end, err = os.OpenFile(path, os.O_WRONLY, 0)
		}
		if err != nil {
			// Nobody opened the other end — the command did not use the path
			// it was given. There is nothing to run and nothing to report:
			// `echo <(true)` prints a path and is not an error anywhere.
			return
		}
		defer func() { _ = end.Close() }()
		if kind == syntax.ProcSubstOut {
			sub.Stdin = end
		} else {
			sub.Stdout = end
		}
		if _, err := sub.Run(ctx, f); err != nil {
			sub.diagf("%v\n", err)
		}
	}()

	r.procSubs = append(r.procSubs, path)
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
	r.procSubHome.once.Do(func() {
		r.procSubHome.dir, r.procSubHome.err = os.MkdirTemp("", "sh-procsub")
	})
	return r.procSubHome.dir, r.procSubHome.err
}

// takeProcSubs hands over the pipes a command's substitutions made, and
// forgets them.
//
// Taken rather than read, because they belong to one command: the next one has
// its own, and a path left on the runner would be removed after some later
// command that never mentioned it.
func (r *Runner) takeProcSubs() []string {
	paths := r.procSubs
	r.procSubs = nil
	return paths
}

// removeProcSubs takes away what a command's substitutions left behind.
//
// The pipe is removed as soon as the command that named it is done. Whatever
// is still reading or writing it holds an open file and does not care that the
// name is gone, which is the property that makes this safe to do early rather
// than at exit — a long session would otherwise fill its directory.
func removeProcSubs(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// CleanUp removes what this shell made for itself.
//
// Only the directory the named pipes went in, at present. A caller that runs
// many scripts on one Runner should call it when finished; a binary that exits
// need not, since the directory is under the system's temporary one.
func (r *Runner) CleanUp() {
	if r.procSubHome != nil && r.procSubHome.dir != "" {
		_ = os.RemoveAll(r.procSubHome.dir)
	}
}
