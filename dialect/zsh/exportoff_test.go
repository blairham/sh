// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// zsh has no `export -n` and is the one shell of the three without it that
// carries on: `export` fails at 1, nothing it was asked to do happens, and
// the next command runs. The letter earns the ordinary `bad option` refusal
// rather than the ExportFunctionOptionRefused wording, because unlike `-f`
// this is a letter zsh has simply never heard of. Measured 2026-09-05 on
// 5.9.2 (#516).
func TestExportHasNoOffOption(t *testing.T) {
	if got := zsh.Semantics().ExportTakesTheAttributeOff; got != interp.No {
		t.Errorf("ExportTakesTheAttributeOff = %v, want No", got)
	}
	out, st := runZsh(t, t.TempDir(), `export -n V; echo "st=$?"; echo alive`)
	want := "zsh:export:1: bad option: -n\nst=1\nalive\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0 — the script survives the refusal here", st)
	}
	if strings.Contains(out, "invalid option(s)") {
		t.Error("got the -f wording for -n, want the ordinary unknown-option refusal")
	}
}
