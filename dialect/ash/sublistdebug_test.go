// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// BusyBox ash has no DEBUG condition either, and this is measured rather
// than taken from dash. In the alpine image internal/oracle.Panel pins —
// sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0 — `trap 'echo T@$LINENO' DEBUG` answers `trap: line 1:
// DEBUG: invalid signal specification` and `echo x && echo y` runs with
// nothing fired, 2026-09-25. An EXIT trap in the same container run is the
// control and does fire.
//
// So the axis keeps its zero value, as a question never put rather than an
// unanswered axis: interp.DebugTrapSublist has no unspecified member. Pinned
// here so that a later change to the zero value is noticed.
func TestSublistDebugAxis(t *testing.T) {
	if got, want := ash.Semantics().DebugTrapSublists, interp.DebugTrapSublistPerOperand; got != want {
		t.Errorf("DebugTrapSublists = %v, want %v", got, want)
	}
}
