// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// This shell's `enable` has no `-d`, so the refusal is about the **letter**.
//
// Measured 2026-09-18 on zsh 5.9.2 with `-f`: `enable -d notbuiltin` is
// `enable: bad option: -d` at 1, where bash answers about the operand. The
// axis is what keeps one column's letter out of the other's option table —
// see Semantics.EnableUnloadsABuiltin.
func TestEnableHasNoDeleteLetter(t *testing.T) {
	if got := zsh.Semantics().EnableUnloadsABuiltin; got != interp.No {
		t.Errorf("EnableUnloadsABuiltin is %v, want No", got)
	}
	out, st := runZsh(t, t.TempDir(), "enable -d notbuiltin\n")
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	if !strings.Contains(out, "bad option") {
		t.Errorf("said %q, want a complaint about the letter", out)
	}
	if strings.Contains(out, "not a shell builtin") || strings.Contains(out, "not dynamically loaded") {
		t.Errorf("answered about the operand: %q", out)
	}
}
