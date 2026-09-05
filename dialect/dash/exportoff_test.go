// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// dash has no `export -n`, so the letter is refused as an unknown option —
// and `export` is a special builtin here, so the script ends there. Measured
// 2026-09-05: `export: Illegal option -n`, status 2, nothing after it runs
// (#516).
func TestExportHasNoOffOption(t *testing.T) {
	if got := dash.Semantics().ExportTakesTheAttributeOff; got != interp.No {
		t.Errorf("ExportTakesTheAttributeOff = %v, want No", got)
	}
	out, st := runDash(t, t.TempDir(), `export -n V; echo alive`)
	if want := "dash: 1: export: Illegal option -n\n"; out != want {
		t.Errorf("got %q, want %q — and nothing after it", out, want)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}
