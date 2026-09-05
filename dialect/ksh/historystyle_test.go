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
func TestHistoryStyleSaysNothing(t *testing.T) {
	if got := ksh.HistoryStyle(); got != (repl.HistoryStyle{}) {
		t.Errorf("HistoryStyle = %+v, want the zero value", got)
	}
}
