// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package plugin

import "os/exec"

// setProcessGroup has nothing to arrange on a platform with no process
// groups, and killGroup falls back to the process alone — so what a plugin
// started outlives the kill there, which is the reachable difference.
//
// Neither is in anything this repository ships: .goreleaser.yaml builds linux
// and darwin. They exist so the package compiles rather than as a claim that
// it works elsewhere.
func setProcessGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
