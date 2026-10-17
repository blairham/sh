// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package acpboot

import (
	"os"
	"os/exec"
)

// With no process groups there is only the agent itself to end, so what it
// started outlives it here — the reachable difference from the unix file.
//
// Nothing this repository ships takes this path: .goreleaser.yaml builds linux
// and darwin. It exists so the teardown reads the same everywhere rather than
// as a claim that it works elsewhere.
func ownGroup(*exec.Cmd) {}

func signalGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func killGroup(*exec.Cmd) {}

var endSignals = []os.Signal{os.Interrupt}

func signalStatus(os.Signal) int { return 130 }
