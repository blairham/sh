// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts a background command in a process group of its own.
//
// docs/design.md committed to real process groups before any code existed,
// and the reason it was worth deciding early is that it cannot be added
// afterwards: a shell whose jobs are goroutines cannot deliver a signal to a
// job, cannot hand the terminal to one, and cannot answer `set -m` honestly.
// Those answers are properties of the process group, not of the bookkeeping
// around it.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}
