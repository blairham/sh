// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// TestNoClobberOverrideMarker pins the side of #1247 that is easy to lose. The
// marker's `!` spelling does not fail here — it is read as a filename, so
// `echo hi >! f` writes a file called `!` holding `hi f` and reports 0.
// Turning the flag on for a dialect that does not have it would silently
// change what that text means rather than starting to accept something new,
// which is why it is asserted off rather than left to the zero value.
func TestNoClobberOverrideMarker(t *testing.T) {
	if dash.Dialect().ClobberOverrideMarker {
		t.Error("`>!` here is `>` and a file named `!`, not the clobber override")
	}
}

// TestNoclobberDoesNotBlockAnAppendThatCreates is the semantics half. POSIX
// 2.7.2 puts the option on `>` alone, and this shell complies: `set -C; echo
// hi >> f` on a missing `f` creates it and reports 0.
func TestNoclobberDoesNotBlockAnAppendThatCreates(t *testing.T) {
	if got := dash.Semantics().NoclobberBlocksAppendCreate; got != interp.No {
		t.Errorf("NoclobberBlocksAppendCreate = %v, want No: noclobber is about `>` here", got)
	}
}
