// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// ksh93 has no `export -n` either, and refuses the letter with the usage line
// under it — then stops the script, `export` being a special builtin.
// Measured 2026-09-05 on 93u+ 2012-08-01 (#516).
func TestExportHasNoOffOption(t *testing.T) {
	if got := ksh.Semantics().ExportTakesTheAttributeOff; got != interp.No {
		t.Errorf("ExportTakesTheAttributeOff = %v, want No", got)
	}
	out, st := runKsh(t, t.TempDir(), `export -n V; echo alive`)
	want := "ksh: export: -n: unknown option\nUsage: export [-p] [name[=value]...]\n"
	if out != want {
		t.Errorf("got %q, want %q — and nothing after it", out, want)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}
