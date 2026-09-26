// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// dash has no DEBUG condition, so an `&&`/`||` list has no firing to count
// and the question #4556 asks is never put here. Measured 2026-09-25 on
// `/bin/dash` — `go version -m` on it says *not a Go executable* — where
// `trap 'echo T@$LINENO' DEBUG` answers `trap: DEBUG: bad trap` and `echo x
// && echo y` then runs with nothing fired. The control in the same run is an
// EXIT trap, which this shell sets and runs, so the instrument that reported
// the refusal is one that can fire.
//
// The axis therefore keeps its zero value rather than being left unanswered:
// interp.DebugTrapSublist has no unspecified member, for the reason
// interp.DebugTrapPipeline has none — a list fires once or once per operand
// and there is no third thing for a shell to mean. Pinned here so that a
// later change to the zero value is noticed.
func TestSublistDebugAxis(t *testing.T) {
	if got, want := dash.Semantics().DebugTrapSublists, interp.DebugTrapSublistPerOperand; got != want {
		t.Errorf("DebugTrapSublists = %v, want %v", got, want)
	}
}
