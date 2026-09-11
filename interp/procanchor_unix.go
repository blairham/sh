// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os"
	"os/exec"
	"syscall"
)

// startProcAnchor starts the placeholder that leads a substitution body's
// process group, and answers with the pipe whose closing ends it.
//
// Three things about the child, each of which is the point rather than tidying:
//
//   - `Setpgid` with no `Pgid` makes it a group *leader*, so the group's id is
//     its own process id. That is what makes the number this shell hands a
//     script both a process it can signal and a group it can signal.
//   - its standard input is a pipe this shell holds the other end of, which is
//     the whole of its lifetime: it reads, reaches end-of-file when that end is
//     closed, and exits. A shell that died without closing anything closes it
//     too, so the anchor never outlives the session.
//   - it is given nothing else. No output, no descriptors past the first, and
//     no environment, so that a placeholder cannot print, cannot hold a
//     terminal, and cannot inherit whatever the shell happened to have open.
func startProcAnchor(argv []string) (*exec.Cmd, *os.File, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = read
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// An empty slice rather than nil: nil is os/exec's spelling for "this
	// process's environment", which is the one thing a placeholder must not
	// be given.
	cmd.Env = []string{}
	err = cmd.Start()
	// This shell's copy of the reading end has done its work either way — the
	// child has it now, or there is no child.
	_ = read.Close()
	if err != nil {
		_ = write.Close()
		return nil, nil, err
	}
	return cmd, write, nil
}

// procGroupExists reports whether a process group still holds anything.
//
// Signal 0 is the ask-without-sending spelling, and the negative number makes
// it the group rather than one process — so this is precisely the question the
// kernel is about to answer for `setpgid`, asked a moment earlier.
//
// It is asked because a script may **kill the group it was given**, which is
// what it asked for the number to do. A real shell's body would have died with
// it; this one is a goroutine and carries on, and a command it started
// afterwards would have failed to join a group that is no longer there —
// `operation not permitted`, for a script that did nothing wrong. Where the
// group has gone the command runs in the shell's, which is where every command
// in a substitution's body ran before there were any groups at all.
//
// The window between this answer and the fork is real and cannot be closed
// from here. It is the same race a real shell has with a signal arriving
// between its own fork and exec, and it is narrow where this was certain.
func procGroupExists(pgid int) bool {
	return syscall.Kill(-pgid, 0) == nil
}

// setProcessGroupIn puts a command in a process group that already exists,
// where setProcessGroup puts it in one of its own.
//
// The two are the same call and differ in one field, which is the difference
// between "lead a group" and "join one" — and joining is what a command
// started inside a process substitution's body does, because a real shell's
// fork would have put it in the body's group without anybody deciding.
func setProcessGroupIn(cmd *exec.Cmd, pgid int) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pgid = pgid
}
