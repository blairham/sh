// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/repl"
)

// This shell answers nothing about either half, and the zero value says so.
//
// Measured on 2026-09-05: ksh93's `C-r` is not an incremental search — it
// echoes `^R` and takes a whole string afterwards — and neither it nor dash
// has a knob for what to leave out; dash keeps no history at all. A name a
// shell does not have must not be half-implemented, so nothing is named here
// and the substrate's own answers stand.
//
// **The default file is not one of those two halves**, and this row is
// narrowed to its own subject rather than loosened. ksh93 does have a file it
// records in when `HISTFILE` is unset — `$HOME/.sh_history`, measured
// 2026-09-30 with an empty home and no rc — so a blanket zero-value check
// would be asserting that a shell has no answer it demonstrably has. Every
// *other* field stays pinned, so a field added in future is still caught
// here; the name itself is graded across the panel in
// dialect.TestEachDialectsDefaultHistoryFile, where it is meaningful as a set.
func TestHistoryStyleSaysNothing(t *testing.T) {
	got := ksh.HistoryStyle()
	if got.DefaultFile == "" {
		t.Error("DefaultFile is empty; ksh93 records in $HOME/.sh_history")
	}
	got.DefaultFile = ""
	if got != (repl.HistoryStyle{}) {
		t.Errorf("HistoryStyle = %+v, want the zero value apart from DefaultFile", got)
	}
}
