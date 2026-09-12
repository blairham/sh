// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package suite

import (
	"os"
	"os/exec"
)

// setProcessGroup does nothing where there are no process groups.
func setProcessGroup(*exec.Cmd) {}

// killGroup ends the process alone. Whatever it started outlives it, which is
// why this instrument's targets are a Unix's.
func killGroup(p *os.Process) {
	if p != nil {
		_ = p.Kill()
	}
}
