// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "os"

// The layout constants firstExtraFd and maxInheritedFd live in
// inheritedfds.go, because they describe both directions of the same table.

// childFiles rebuilds, for whatever this shell is about to hand the table to,
// the descriptors it holds beyond the three named streams.
//
// Two things read it, and they want the same answer. An external command is
// given it by number through os/exec's ExtraFiles; a *replacement* — `exec
// cmd`, the path that becomes the command rather than forking one — is given
// it by ReplaceProcess, which places each entry on its number before execve.
// The second is not a child and still inherits exactly what a child does:
// `exec 3>h; exec /bin/sh -c 'echo repl >&3'` writes in bash, dash and zsh
// just as the un-replaced form does, and the gaps and the exclusions below
// are the same there too.
//
// The boundary has to be rebuilt by hand because Go opens everything
// close-on-exec, so a descriptor the script put on 3 with `exec 3>f` reaches
// no child at all: `exec 3>f; /bin/sh -c 'echo child >&3'` wrote nothing and
// said "Bad file descriptor". Every shell in the panel but ksh93 hands the
// descriptor over, which is what the flock and shared-log idioms rely on.
//
// The layout is the whole of it, and it is measured rather than assumed:
// with 3 and 4 unopened, `exec 5>f; /bin/sh -c 'echo x >&5'` writes in bash,
// dash and zsh, and 3 and 4 are *closed* in that child rather than shifted
// down. So the number is preserved and a gap is a hole: entry fd-3 holds the
// file, and a nil entry is a descriptor closed in the child, which is exactly
// what os/exec does with a nil.
//
// Only a real file can cross. A Runner embedded in another program may have a
// buffer or a pipe of the caller's behind a descriptor, and there is no
// number to hand a child for one of those; those entries stay nil, which
// leaves them closed there as they were before.
//
// A descriptor this shell *inherited* and has since closed reaches as high as
// one it opened, and for the sake of the nil rather than the file. The
// original is still open in this process and is not close-on-exec — that is
// how it was recognized — so it would otherwise pass to a child through the
// kernel, behind the table's back: `exec 3<&-` then an external command found
// 3 still readable there, where all four shells find it closed. Reaching the
// number puts a nil at it, and a nil is a close.
// tableFile is the descriptor a table entry stands for, and nil where there
// is none — an embedder's buffer, a here-document's text, a coprocess's near
// end that childFiles is not to rebuild.
//
// A fan of several targets or sources answers with the *last* of them, which
// is what the number held before the fan existed. See fanSource for why that
// is the right wrong answer rather than closing the number in the child.
func tableFile(v any) *os.File {
	switch f := v.(type) {
	case *os.File:
		return f
	case multiTarget:
		return f.last
	case fanSource:
		return f.last
	}
	return nil
}

func (r *Runner) childFiles() []*os.File {
	highest := 0
	for i, f := range r.InheritedFiles {
		if fd := firstExtraFd + i; f != nil && fd <= maxInheritedFd && fd > highest {
			highest = fd
		}
	}
	for fd, v := range r.fds {
		if fd < firstExtraFd || fd > maxInheritedFd {
			continue
		}
		if tableFile(v) != nil && fd > highest {
			highest = fd
		}
	}
	if highest == 0 {
		return nil
	}
	files := make([]*os.File, highest-firstExtraFd+1)
	for fd, v := range r.fds {
		if fd < firstExtraFd || fd > highest {
			continue
		}
		if f := tableFile(v); f != nil {
			files[fd-firstExtraFd] = f
		}
	}
	r.dropExecOpened(files)
	r.dropCloseOnExec(files)
	return files
}

// dropCloseOnExec takes out the descriptors a builtin was told to keep from
// what this shell runs.
//
// A nil rather than a gap, for the reason dropExecOpened gives: the number
// must be *closed* in the child, and a nil is what says so on both routes out
// of this table.
//
// No axis is asked, which is the difference from its neighbor. `exec 3>f`
// leaves a shell to decide whether the descriptor is the script's or its own,
// and the shells disagree; `sysopen -o cloexec` is the script having decided,
// by name, on that line.
func (r *Runner) dropCloseOnExec(files []*os.File) {
	for fd := range r.cloexecFds {
		if i := fd - firstExtraFd; i >= 0 && i < len(files) {
			files[i] = nil
		}
	}
}

// replacementFiles is the whole of the table a *replacement* is given: the
// three named streams as well as everything childFiles rebuilds above them.
//
// A replacement needs the wider answer because it is the one route that has no
// renumbering step. An external child is handed 0, 1 and 2 by os/exec, which
// will copy bytes through a pipe for a stream that is not a file at all; `exec
// cmd` becomes the command in this process, so whatever the process's own 0, 1
// and 2 say at the moment of the execve is what the command gets. So `exec
// >log; exec /bin/echo hi` wrote to the terminal and `exec <data; exec
// /bin/cat` read the caller's input, in both cases past a redirection the
// script had already applied. All five shells give the replacement the
// redirected stream.
//
// The layout is childFiles' layout extended down to zero — entry i is
// descriptor i — rather than childFiles' own, and that is why it is a slice of
// its own rather than an extra field: the ExtraFiles layout starts at 3
// because os/exec carries the named streams separately, and there is nothing
// to carry them separately *with* here.
//
// Both of the rules the table already states carry over unchanged, and the
// second one is a decision rather than a consequence:
//
//   - Only a real file can cross. A Runner embedded in another program may
//     have a caller's buffer behind standard output, and one dialect's
//     `exec >a >b` writes to both files at once, which is no single file
//     either. There is no number to hand a replacement for one of those.
//   - A nil is a number that must not be open there. It says *closed* for
//     the named streams as it does for the rest of the table, rather than
//     "leave the process's own": a script that says `exec >&-` and then
//     `exec cmd` leaves the command a closed descriptor in every shell in
//     the panel — measured, unanimous, and the status is the command's
//     complaint rather than its work. Leaving the process's own stream would
//     also be the one way for an embedder's terminal to reach a command
//     through a stream the script had redirected away from, which is the
//     borrowing of process state this package exists not to do.
//
// The exclusion `exec` makes above 2 stops there, and that is measured too:
// the one shell that keeps a descriptor its own `exec` opened to itself hands
// over a redirected standard stream like every other shell.
func (r *Runner) replacementFiles() []*os.File {
	extra := r.childFiles()
	files := make([]*os.File, firstExtraFd+len(extra))
	copy(files[firstExtraFd:], extra)
	files[0] = streamFile(r.Stdin)
	files[1] = streamFile(r.Stdout)
	files[2] = streamFile(r.Stderr)
	return files
}

// streamFile is the file behind one of the named streams, and nil where the
// stream is not one.
//
// The coprocess wrapper is unwrapped here, which is the one place that mark
// does not travel. It exists so that a coprocess's near ends are not rebuilt
// into the table a command inherits, and the table it guards starts at 3:
// `exec >&${C[1]}` puts the feed on standard output, and the command a
// replacement runs writes into the coprocess there — measured, and true of an
// external child in the same shell. Our child route reaches the same place by
// a different road, because os/exec copies bytes for a stream that is not a
// file; a replacement has no such road, so the number has to be real.
func streamFile(v any) *os.File {
	switch f := v.(type) {
	case *os.File:
		return f
	case shellOwnedFd:
		return f.File
	}
	return nil
}

// dropExecOpened takes back the descriptors `exec` opened, where the dialect
// says they are the shell's alone.
//
// A dialect question rather than a rule: four of the five hand them over, and
// one closes anything above 2 that `exec` opened before it runs anything —
// see ExecOpenedFdReachesACommand, which carries the measurements and the
// boundary. Nothing else in the table is touched, which is the whole of the
// difference: a descriptor the caller opened, and one this command redirected
// itself, are not `exec`'s and cross in every shell.
//
// The axis is asked only when the table actually holds one. An axis consulted
// on the common path would refuse every external command in a Runner that had
// not chosen a dialect, over a question that decides nothing for a script with
// no parked descriptors.
func (r *Runner) dropExecOpened(files []*os.File) {
	held := false
	for fd := range r.execFds {
		if i := fd - firstExtraFd; i >= 0 && i < len(files) && files[i] != nil {
			held = true
			break
		}
	}
	if !held {
		return
	}
	if r.ask(r.sem().ExecOpenedFdReachesACommand,
		"a descriptor `exec` parked reaching what the shell runs") {
		return
	}
	for fd := range r.execFds {
		if i := fd - firstExtraFd; i >= 0 && i < len(files) {
			// A nil rather than a gap: the number must be *closed* there, and
			// a nil is what says so on both routes out of this table.
			files[i] = nil
		}
	}
}
