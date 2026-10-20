// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The other side of the split: `exit` inside a function unwinds the call
// before the EXIT trap runs here, so the body reads the global.
//
// Measured 2026-09-21 on zsh 5.9.2, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME. See
// interp.Semantics.ExitTrapRunsInsideTheExitingCall.
func TestTheExitTrapDoesNotSeeTheCallThatExited(t *testing.T) {
	const src = "v=global\ng() { local v=inner; trap 'echo \"v=$v\"' EXIT; exit 0; }\ng"
	out, st := answersRun(t, src)
	if got := strings.TrimSpace(out); got != "v=global" {
		t.Errorf("output %q, want %q", got, "v=global")
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}
