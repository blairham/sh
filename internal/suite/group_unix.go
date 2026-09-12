// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package suite

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup puts the run in a process group of its own, so that
// everything it starts can be ended together.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup ends the whole group. The negative pid is the group, and the
// group is the leader's pid because setProcessGroup made it one.
//
// SIGKILL rather than SIGTERM: this is reached only after the timeout, the
// file being killed is one that did not finish, and a handler that caught a
// term would be one more thing between here and a bounded run.
func killGroup(p *os.Process) {
	if p == nil {
		return
	}
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
	_ = p.Kill()
}
