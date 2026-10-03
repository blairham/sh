// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package interp

// childIsGone answers false where there is no signal 0 to ask with, which
// leaves the job to the reaping signal alone. See Runner.awaitAReapedProgramsJob.
func childIsGone(int) bool { return false }
