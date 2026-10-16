// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

import "os/exec"

// claimStartLeftovers has no numbers to watch where a child is not handed
// descriptors by number. See foreignfds_unix.go.
func claimStartLeftovers(_ *exec.Cmd, start func() error) error { return start() }

// dropFdClaimMade has no claim to drop. See foreignfds_unix.go.
func dropFdClaimMade(int, uint64) {}
