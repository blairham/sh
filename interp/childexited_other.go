// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !linux && !darwin

package interp

// childHasExited answers false where there is no waitid to peek with, which
// leaves an unreaped child to `kill -0` and the reaping signal, as before. See
// Runner.awaitAReapedProgramsJob.
func childHasExited(int) bool { return false }
