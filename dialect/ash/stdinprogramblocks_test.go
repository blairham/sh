// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// A program arriving on standard input is taken in blocks rather than a line
// at a time, which is dash's answer and not bash's.
//
// Measured 2026-09-21 in the digest-pinned image:
//
//	printf 'read x\necho "[$x]"\nDATA\n' | ash
//
// prints `[]` and then fails to find `DATA` as a command, exactly as dash
// does — the shell had already swallowed the rest of the descriptor, so
// `read` found end of input. bash, ksh93 and zsh hand the second line to
// `read` and never parse it, which prints nothing and runs `DATA` at line 3.
//
// This preset held bash's answer for the whole life of the column, and the
// axis doc named dash alone, so nothing in the tree disagreed with anything
// else. It was found by reading the four-shell prose of #3228 against the
// shell rather than by a test, which is the campaign's second live defect.
func TestAshTakesAProgramOnStandardInputInBlocks(t *testing.T) {
	if !ash.Semantics().StdinProgramReadInBlocks {
		t.Error("StdinProgramReadInBlocks = false, want true")
	}
	// And the substrate's own answer is still the line, so this is the
	// preset saying something rather than inheriting it.
	if (interp.Semantics{}).StdinProgramReadInBlocks {
		t.Error("the substrate's own answer is blocks, want the line")
	}
}
