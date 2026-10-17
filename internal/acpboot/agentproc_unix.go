// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package acpboot

import (
	"os"
	"os/exec"
	"syscall"
)

// ownGroup starts the agent as the leader of a process group of its own.
//
// An agent is rarely one process. The registry launches them through npx, so
// the child this shell starts is `npm exec` and the one speaking the protocol
// is a node process under it; signalling the child alone leaves that one
// running, orphaned, still holding the pipe. A group is what lets the shell
// end everything the agent started with one signal.
func ownGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalGroup asks the agent's whole group to end. SIGTERM, because this is
// the polite half: it is sent once the agent has had its grace period after
// end of input, and a well-behaved agent gets to tidy up.
func signalGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	// The negative pid is the group, and Setpgid made the group id the pid.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	return nil
}

// killGroup ends whatever is left of the agent's group.
//
// Called after the agent has been waited for, whether it left by itself or
// not: a leader that exits on end of input can still leave a grandchild
// behind it, and that grandchild is exactly the process nobody would think to
// look for. SIGKILL, because a process that has outlived both end of input and
// a SIGTERM is not going to honour a third request. A group that is already
// empty answers ESRCH, which is the outcome wanted.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// endSignals are the signals that end a session early. They are caught,
// rather than left to kill this process, because the agent is no longer in
// this process's group: a ^C at the terminal reaches the shell and not the
// agent, and a shell that died of it would leave the agent behind.
var endSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// signalStatus is the status a shell reports for dying of sig.
func signalStatus(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}
