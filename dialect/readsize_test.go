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

// dash, ksh93 and BusyBox ash end the prompt's line at the end of the input;
// bash says its word instead and zsh says nothing. Measured 2026-10-07 on a
// pipe (#6330).
func TestTheEndOfInputEndsTheLineInDashAshAndKsh(t *testing.T) {
	for _, c := range []struct {
		name string
		sem  interp.Semantics
		want bool
	}{
		{"dash", dash.Semantics(), true},
		{"ash", ash.Semantics(), true},
		{"ksh", ksh.Semantics(), true},
		{"bash", bash.Semantics(), false},
		{"zsh", zsh.Semantics(), false},
	} {
		if got := c.sem.PromptEndOfInputEndsTheLine; got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// ash draws again the prompt an unterminated last line was read at, ksh93
// draws the continuation prompt, and the others draw nothing. Measured
// 2026-10-07 on a pipe (#6331).
func TestAnUnterminatedLastLineIsPromptedForAsMeasured(t *testing.T) {
	for _, c := range []struct {
		name string
		sem  interp.Semantics
		want interp.UnterminatedLinePrompt
	}{
		{"ash", ash.Semantics(), interp.SamePromptForAnUnterminatedLine},
		{"ksh", ksh.Semantics(), interp.ContinuationPromptForAnUnterminatedLine},
		{"dash", dash.Semantics(), interp.NoPromptForAnUnterminatedLine},
		{"bash", bash.Semantics(), interp.NoPromptForAnUnterminatedLine},
		{"zsh", zsh.Semantics(), interp.NoPromptForAnUnterminatedLine},
	} {
		if got := c.sem.PromptAgainForAnUnterminatedLine; got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// dash, ksh93 and BusyBox ash read a backslash a prompt's input ends on as a
// word; bash and zsh as a continuation the end finishes. Measured 2026-10-07
// on a pipe (#6337).
func TestABackslashTheInputEndsOnIsLiteralAsMeasured(t *testing.T) {
	for _, c := range []struct {
		name string
		sem  interp.Semantics
		want bool
	}{
		{"dash", dash.Semantics(), true},
		{"ash", ash.Semantics(), true},
		{"ksh", ksh.Semantics(), true},
		{"bash", bash.Semantics(), false},
		{"zsh", zsh.Semantics(), false},
	} {
		if got := c.sem.PromptBackslashTheInputEndsOnIsLiteral; got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
