// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// Nothing is seeded into an absent `HOME` at startup here, and a `HOME` that
// has been removed is an absent one rather than an empty one — #4654.
//
// Measured 2026-09-26 in the panel's own image,
// `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`,
// BusyBox v1.37.0. `cd` with no home at all is 0 and prints nothing, and so is
// `HOME=/tmp; unset HOME; cd` — which is exactly why the second axis cannot be
// measured in this column at all.
//
// See interp.Semantics.StartupFillsAnAbsentHome and
// interp.Semantics.CdRemembersAHomeThatWasUnset.
func TestTheShellHoldsNoHomeOfItsOwn(t *testing.T) {
	if got := ash.Semantics().StartupFillsAnAbsentHome; got != interp.No {
		t.Errorf("StartupFillsAnAbsentHome = %v, want interp.No", got)
	}
	if got := ash.Semantics().CdRemembersAHomeThatWasUnset; got != interp.No {
		t.Errorf("CdRemembersAHomeThatWasUnset = %v, want interp.No", got)
	}
	// And the second is not a measurement: `CdWithoutHomeIsAnError` is No
	// here, so a `cd` with no home at all is already a silent 0 and both
	// readings of the axis produce the same row on every line of its table.
	// The control is that first line — if it ever became Yes, this column
	// would have something to measure and this test would be asserting a
	// value nobody had.
	if got := ash.Semantics().CdWithoutHomeIsAnError; got != interp.No {
		t.Errorf("CdWithoutHomeIsAnError = %v, want interp.No — "+
			"this column can now tell the two readings apart and the axis above needs measuring", got)
	}
}
