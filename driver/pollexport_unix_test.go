// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

import "github.com/blairham/sh/interp"

// PollCommandForTest is pollCommand, reachable from the package's external
// tests.
//
// Exposed because the third answer is the whole of it: a blocking wait has
// only "stopped", "signaled" and "exited", and this one also has "nothing has
// happened" — which is what a shell asking after a job it is not waiting for
// gets almost every time. Nothing a Shell can be handed makes that visible,
// and a poll that reported a change where there was none would finish a
// running job.
func PollCommandForTest(pid int) (interp.Wait, bool, error) { return pollCommand(pid) }
