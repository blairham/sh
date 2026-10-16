// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// Which columns end the session over a `return` typed at a prompt with
// nothing to return from — Semantics.ReturnAtThePromptEndsTheSession, and the
// repl's half of it is TestAReturnAtThePromptEndsTheLineOrTheSession.
//
// Measured 2026-10-16 through a pseudo-terminal with a two-row PS1: ksh93u+
// and dash end at the `return`'s status with the EXIT trap run, zsh 5.9.2
// costs the line and reads on (#6717). BusyBox ash 1.37.0 ends there too, on
// standard input of `sh -i`. bash refuses the `return` before this is asked.
func TestReturnAtThePromptEndsTheSessionPerDialect(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  func() interp.Semantics
		want interp.Answer
	}{
		{"zsh", zsh.Semantics, interp.No},
		{"ksh", ksh.Semantics, interp.Yes},
		{"dash", dash.Semantics, interp.Yes},
		{"ash", ash.Semantics, interp.Yes},
	} {
		if got := tc.sem().ReturnAtThePromptEndsTheSession; got != tc.want {
			t.Errorf("%s: ReturnAtThePromptEndsTheSession = %v, want %v", tc.name, got, tc.want)
		}
	}
}
