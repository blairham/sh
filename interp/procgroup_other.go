// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

import "os/exec"

// setProcessGroup does nothing where process groups do not exist.
//
// Windows has job objects rather than process groups, and mapping one onto
// the other is a design question rather than a translation. Doing nothing is
// the honest placeholder: background commands still run, and the answers that
// depend on grouping are simply not available yet.
func setProcessGroup(cmd *exec.Cmd) {}
