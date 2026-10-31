// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"reflect"
)

// Where a stream a `read` names came from, which one column reads off when a
// trapped signal arrives during the read: ksh93 gives a read of a stream it
// opened at the number the read names up at the next byte, and reads a copy
// the way it reads its standard input — only at the start of a line (#7030).
// See readWatch.atTheNextByte.
//
// Neither fact is about the stream itself. A copy is the same stream under a
// second number, so what tells `exec 3<fifo; read -u3` from `exec 4<&3; read
// -u4` is the entry and not what is in it; and a builtin's own `<fifo` opens a
// stream like any other, which `exec 0<fifo` does too.

// duplicated reports that descriptor fd's entry was made by copying another —
// `exec 4<&3`, `exec 3<&3`, `{v}<&3`, or a command's own `4<&3` — rather than
// opened at fd, inherited there, or moved there from an entry that was not a
// copy. It stays a copy once its source is closed.
//
// Kept beside the table and on its terms: setFd takes the mark off, since any
// other write to a number is a different descriptor; the save a command's
// redirections take puts it back with the table; and a subshell copies it.
func (r *Runner) duplicated(fd int) bool { return r.dupFds[fd] }

// markDuplicated records whether descriptor fd's entry is a copy. Only the
// numbers past the three named streams are in the table, and only theirs are
// asked about: a read that names 0 is a read of standard input.
func (r *Runner) markDuplicated(fd int, dup bool) {
	switch {
	case fd <= 2:
	case !dup:
		delete(r.dupFds, fd)
	default:
		if r.dupFds == nil {
			r.dupFds = map[int]bool{}
		}
		r.dupFds[fd] = true
	}
}

// noteOpenedStdin records that the redirection list being applied opened a
// file as standard input, for simpleCommand to hand to the builtin it runs.
// Recorded only where the stream can be compared, like ownStdin: one that
// cannot is never matched, and its read is read as standard input.
func (r *Runner) noteOpenedStdin() {
	if in := r.Stdin; in != nil && reflect.TypeOf(in).Comparable() {
		r.openedStdin = in
	}
}

// holdBuiltinStdin makes in, which the command's own redirection opened as its
// standard input, the stream a builtin reads as one it opened, for as long as
// the command runs; the function it returns puts the last one back.
//
// A builtin and not a function, and for everything the builtin runs:
// measured 2026-10-31 on ksh93u+, with a fifo, `read l <fifo`, `command read
// l <fifo`, `eval 'read l' <fifo` and `eval f <fifo` are given up at the next
// byte, and `f <fifo` and `{ eval 'read l'; } <fifo` read on — as `exec
// 0<fifo; read l` does, where the open outlives the command that made it.
func (r *Runner) holdBuiltinStdin(argv []string, in io.Reader) func() {
	if len(argv) == 0 {
		return func() {}
	}
	if _, isFunc := r.funcs[argv[0]]; isFunc {
		return func() {}
	}
	if _, isBuiltin := r.lookupBuiltin(argv[0]); !isBuiltin {
		return func() {}
	}
	before := r.builtinStdin
	r.builtinStdin = in
	return func() { r.builtinStdin = before }
}
