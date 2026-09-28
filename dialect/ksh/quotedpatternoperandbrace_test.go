// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// Balancing a `{ … }` inside a **double-quoted** parameter expansion's
// pattern operand is this shell's alone.
//
// Measured 2026-09-27 from `-c` under `env -i PATH=/usr/bin:/bin`:
// `s=x{y}z; echo "[${s#x{y}}]"` is `[z]` on `/bin/ksh`
// `Version AJM 93u+ 2012-08-01` and `[}z}]` on `/opt/homebrew/bin/bash`
// 5.3.20, `/bin/bash` 3.2.57, `/opt/homebrew/bin/zsh` 5.9.2 and `/bin/dash`.
// `go version -m` says *not a Go executable* for each.
//
// The **unquoted** reading is a different flag and two columns have it, so
// the pair is asserted together: a test that only read the new field would
// pass for a dialect that had lost the old one.
func TestABalancedQuotedPatternOperandIsKshsAlone(t *testing.T) {
	if !ksh.Dialect().BareBraceNestsInAQuotedPatternOperand {
		t.Error("ksh: BareBraceNestsInAQuotedPatternOperand is off, want it on")
	}
	if !ksh.Dialect().BareBraceNestsInExpansion {
		t.Error("ksh: BareBraceNestsInExpansion is off, want it on")
	}
	// zsh has the unquoted reading and not this one, which is the pair that
	// says the two are separate questions rather than one setting.
	if !zsh.Dialect().BareBraceNestsInExpansion {
		t.Error("zsh: BareBraceNestsInExpansion is off, want it on")
	}
	for _, tc := range []struct {
		name string
		d    syntax.Dialect
	}{
		{"bash", bash.Dialect()},
		{"zsh", zsh.Dialect()},
		{"dash", dash.Dialect()},
		{"core", syntax.Core()},
	} {
		if tc.d.BareBraceNestsInAQuotedPatternOperand {
			t.Errorf("%s: BareBraceNestsInAQuotedPatternOperand is on, want it off", tc.name)
		}
	}
}
