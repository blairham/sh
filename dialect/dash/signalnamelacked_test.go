// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
)

// TestThisPresetHasNoNameForStackFault is the other column whose own table is
// shorter than the machine's, and it is the other platform.
//
// Measured 2026-09-17 on dash 0.5.12 in the panel's Alpine image
// (alpine@sha256:28bd5f…), under a matching `argv[0]`:
//
//	kill -l 16        16                          status 0
//	kill -STKFLT 1    kill: Illegal option -S     status 2
//	kill -16 999999   kill: No such process       status 1
//
// where bash 5.3, zsh 5.9 and BusyBox ash all write `STKFLT` for 16. The
// number in range is still sent, which is what separates a missing *name*
// from a number outside the kernel's range — that one is `Illegal option -6`
// at 2 for 65, and it is KillListPrintsANumberItCannotName's question rather
// than this one.
//
// Asserted on the preset alone. The behavior is a Linux fact and this file
// runs on both, so a run here would either skip on macOS or pin the machine
// rather than the dialect; interp's own cases reach the same code through a
// vector missing a name that every platform has.
func TestThisPresetHasNoNameForStackFault(t *testing.T) {
	if got := dash.Semantics().SignalNamesTheShellLacks; got != "STKFLT" {
		t.Errorf("SignalNamesTheShellLacks is %q, want STKFLT", got)
	}
}
