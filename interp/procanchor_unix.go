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
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// An empty slice rather than nil: nil is os/exec's spelling for "this
	// process's environment", which is the one thing a placeholder must not
	// be given.
	cmd.Env = []string{}
	// Not through Runner.startMasked: an anchor is a placeholder with no
	// environment, no output and nothing to run, so there is no file for a
	// file-creation mask to be about. It is also not a Runner's command.
	// It is still a start, and opens descriptors in this process while it
	// runs — see transientFds. And it leaves two behind for the anchor's
	// life, the writing end below and on Linux a pidfd, which are no forking
	// shell's: so the pipe is made inside the same window and both are
	// claimed as nobody's. See claimStartLeftovers.
	var read, write *os.File
	err := withTransientFds(func() error {
		return claimStartLeftovers(nil, func() error {
			var perr error
			if read, write, perr = os.Pipe(); perr != nil {
				return perr
			}
			cmd.Stdin = read
			return cmd.Start()
		})
	})
	// This shell's copy of the reading end has done its work either way — the
	// child has it now, or there is no child.
	if read != nil {
		_ = read.Close()
	}
	if err != nil {
		if write != nil {
			_ = write.Close()
		}
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
//
// Narrower now than when that was written. A negative pid used to be the
// sure way in: a pipeline element running `kill -- -$BASHPID` beside a
// `/bin/cat` killed the group while the cat was being checked into it, and
// the cat failed every time (#7238). The `kill` builtin now refuses a group
// kill aimed at a body forked with the monitor off, as both references do,
// because no shell that forks has that group — see aimedAtAGroupNoForkLeads.
// What still reaches the placeholder is a kill this shell does not answer: a
// group kill under the monitor at a body that is not a job's — a pipeline
// element, a substitution — where the group is a real one, and a signal sent
// from another program, `/bin/kill` included. A job's group is answered by the
// bodies in it and sends nothing (#7246); see jobgroupsignal.go.
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
