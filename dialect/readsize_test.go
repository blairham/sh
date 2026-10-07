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

// How much each shell takes of a descriptor it shares, at a prompt reading a
// pipe and of a program on standard input. Measured 2026-10-07 by placing
// `DATA` at byte P after `read x` and finding the one P whose `read` gets it:
// bash 5.3.20, zsh 5.9.2 and ksh93u+ take a line on both routes; dash 0.5.12
// takes the C library's buffer on both (1024 on macOS, 8192 in Debian);
// BusyBox ash 1.37.0 in the pinned image takes 1024 at a prompt and 2047 of a
// program (#6328, #6329).
func TestEveryDialectReadsWhatItWasMeasuredToRead(t *testing.T) {
	for _, c := range []struct {
		name          string
		sem           interp.Semantics
		prompt, stdin interp.ReadSize
	}{
		{"bash", bash.Semantics(), interp.ReadSizeLine, interp.ReadSizeLine},
		{"zsh", zsh.Semantics(), interp.ReadSizeLine, interp.ReadSizeLine},
		{"ksh", ksh.Semantics(), interp.ReadSizeLine, interp.ReadSizeLine},
		{"dash", dash.Semantics(), interp.ReadSizeCBuffer, interp.ReadSizeCBuffer},
		{"ash", ash.Semantics(), 1024, 2047},
	} {
		if got := c.sem.PromptReadSize; got != c.prompt {
			t.Errorf("%s: PromptReadSize %d, want %d", c.name, got, c.prompt)
		}
		if got := c.sem.StdinProgramReadSize; got != c.stdin {
			t.Errorf("%s: StdinProgramReadSize %d, want %d", c.name, got, c.stdin)
		}
	}
}
