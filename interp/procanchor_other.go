// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

import (
	"errors"
	"os"
	"os/exec"
)

// startProcAnchor refuses where process groups do not exist, which is the same
// line procgroup_other.go takes for the same reason: Windows has job objects
// rather than process groups, and mapping one onto the other is a design
// question rather than a translation.
//
// A body whose anchor cannot start answers "no group", which is the answer
// every body gave before there were any — so nothing here regresses and
// nothing pretends.
func startProcAnchor([]string) (*exec.Cmd, *os.File, error) {
	return nil, nil, errors.ErrUnsupported
}

// procGroupExists answers no, since nothing here ever starts a group.
func procGroupExists(int) bool { return false }

// setProcessGroupIn does nothing, for the same reason setProcessGroup does.
func setProcessGroupIn(cmd *exec.Cmd, pgid int) {}
