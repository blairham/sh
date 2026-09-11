// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"io"
	"os"
)

// A process substitution's body runs on a goroutine here, and a script that
// starts a daemon through one asks the body which process it is so that it can
// signal the whole group later. interp answers that with a real process group
// and needs a real process to lead it — see interp/procanchor.go, which is
// where the argument lives. This file is the front end's half: *which program*
// leads it, and what that program does.
//
// It is this shell's own, and the reason is the reason every other line in the
// KeepProcess block of newRunner is there. Finding this program on disk is
// reading a fact about *the process*, and a Runner embedded in some other
// program would be starting copies of that program instead — so interp takes
// the answer and a binary that is the shell goes looking for it.
//
// Why not something already on the machine: `cat` would do the job and would
// make the shell depend on a command being installed to answer a parameter,
// which is a dependency no shell has. Why not this shell running a script that
// blocks: a shell reads its startup files, and a placeholder that sourced the
// person's `~/.zshenv` would run their code in a process they cannot see,
// invisibly, several times a session. The placeholder must do *nothing*, and
// the only way to be sure of that is for it to be nothing.

// anchorArg is the argument that makes a shell binary a placeholder.
//
// A word rather than an environment variable, which was the other candidate
// and is worse in the way that matters: the environment is inherited, so a
// variable that turned a shell inert would turn every shell started under one
// inert, and a leak would be a machine where nothing runs. An argument reaches
// exactly the process it is given to.
//
// Its spelling says what it is for and is not a shell option in any dialect:
// `-` introduces options everywhere, `--` ends them everywhere, and nothing in
// the panel reads a long word after `--`. Read before anything else, so no
// option loop has to know about it.
const anchorArg = "--hold-process-group"

// holdProcessGroup is the whole of the placeholder: exist, and stop existing
// when the shell that started it says so.
//
// The say-so is end-of-file on standard input, which is the one signal that
// arrives however the shell ends. A shell that closed the pipe deliberately
// and a shell that was killed both deliver it, so the placeholder cannot
// outlive the session that made it — which a timeout, a signal handler or a
// parent-pid poll would each get wrong in a different direction.
//
// It reads and discards rather than blocking on a read that must not return,
// because the pipe is a pipe: nothing is ever written to it, and a write that
// somehow happened must not turn a placeholder into something that has read
// half a line and is waiting for the rest.
func holdProcessGroup() int {
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

// heldProcessGroup reports an argument vector asking for the placeholder, which
// is the whole vector and nothing else. A shell asked to be a placeholder is
// not also asked to run anything.
func heldProcessGroup(argv []string) bool {
	return len(argv) == 2 && argv[1] == anchorArg
}

// processAnchor is the command interp starts to lead a substitution body's
// process group: this program, asked to be a placeholder.
//
// Empty where the program cannot be found, which is a shell whose substitution
// bodies have no group — the state every one of them was in before this
// existed, and one the parameter that reports it already answers for.
func processAnchor() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	return []string{exe, anchorArg}
}
