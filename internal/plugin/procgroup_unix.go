// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package plugin

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts a plugin in a process group of its own, which is the
// same thing interp does for a background command and for the same two
// reasons, both of which matter here.
//
// A ^C at the prompt goes to the foreground process group. A plugin in that
// group would receive it directly, so the host would be racing the terminal
// to decide what a cancellation means — and the host is the only party that
// knows there is a call outstanding.
//
// And killing a group kills what the plugin started. A plugin that spawns a
// helper and is then killed on its own would leave the helper holding the
// descriptors the host is waiting to see closed.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killGroup sends SIGKILL to the plugin's whole process group.
//
// SIGKILL rather than SIGTERM, and that is not impatience: this is only
// reached after the plugin has been given EOF on its input and, for a
// canceled call, a cancellation it did not answer. A signal it can catch is
// a signal it can also ignore, and the host's guarantee that a blocked read
// on the plugin's output will return depends on the process actually going
// away.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	// The negative pid is the group, and Setpgid made the group id the pid.
	// The process itself is killed too if the group signal does not land —
	// which it will not if Start raced with this, and a shell that shut a
	// plugin down and left it running would be worse than either.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
