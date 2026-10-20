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

// A repetition count written in braces in front of a pattern group —
// `{2,3}(a)` — is this shell's and no other's.
//
// Measured 2026-09-27 under `env -i PATH=/usr/bin:/bin`. `echo A{2,3}(a)B`
// is `A{2,3}(a)B` on `/bin/ksh` `Version AJM 93u+ 2012-08-01`; it is
// `syntax error near unexpected token `('` on `/opt/homebrew/bin/bash`
// 5.3.20 with `extglob` on *and* off; `Syntax error: "(" unexpected` on
// `/bin/dash`; and on `/opt/homebrew/bin/zsh` 5.9.2 it is
// `no matches found: A2(a)B` — the brace read as an ordinary list and the
// `(a)` as that shell's own bare group, which is a different construct
// wearing the same characters. `go version -m` says *not a Go executable*
// for each.
//
// So a dialect that took the parenthesis would accept what its own shell
// refuses, and zsh's column would answer a question it is not being asked.
func TestACountInFrontOfAGroupIsKshsAlone(t *testing.T) {
	if !ksh.Dialect().CountedPatternGroup {
		t.Error("ksh: CountedPatternGroup is off, want it on")
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
		if tc.d.CountedPatternGroup {
			t.Errorf("%s: CountedPatternGroup is on, want it off", tc.name)
		}
	}
}
