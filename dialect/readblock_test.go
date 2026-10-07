// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// dash alone throws away the rest of a read block after an error at a prompt
// on a pipe. Measured 2026-10-07 with the input in one write: dash 0.5.12
// loses it, and bash 5.3.20, zsh 5.9.2, ksh93u+ and BusyBox ash 1.37.0 in the
// pinned alpine image run every line (#6322). ash is asked by name because it
// is the one most easily taken to be dash's.
func TestOnlyDashLosesTheRestOfTheReadBlock(t *testing.T) {
	for _, c := range []struct {
		name string
		sem  interp.Semantics
		want bool
	}{
		{"dash", dash.Semantics(), true},
		{"ash", ash.Semantics(), false},
		{"bash", bash.Semantics(), false},
		{"zsh", zsh.Semantics(), false},
		{"ksh", ksh.Semantics(), false},
	} {
		if got := c.sem.PromptErrorDiscardsTheRestOfTheReadBlock; got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
