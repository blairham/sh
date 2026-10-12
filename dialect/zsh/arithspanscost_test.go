// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// A function that opens the way a plugin's do — `emulate -L zsh` and a few
// grammar options — and evaluates an arithmetic expression with an expansion
// in it reads that expression's text once, not once per call.
//
// Two things stood in the way. The options moved the runner's dialect to a
// fresh copy on every call, so nothing keyed on the dialect saw the same
// grammar twice; and the cut of the text into spans, with a parse of every
// `${…}` in it, was redone at every evaluation regardless. F-Sy-H's
// highlighter is this shape, and evaluates `(( ${+seen[$line]} ))` twenty
// times a keystroke (#6423). See interp/dialectmove.go and
// interp/arithspancache.go.
//
// Allocations rather than time, so the row is deterministic.
func TestAPluginShapedCallReadsItsArithmeticOnce(t *testing.T) {
	const defs = "typeset -A A; A=(k v); k=k\n" +
		"f() { emulate -L zsh; setopt rc_quotes no_short_loops; (( ${+A[$k]} )); }\n"
	call := localCostRunner(t, defs, "f")
	call()
	// 27 on the change that set this, and 54 on main before it.
	if got := testing.AllocsPerRun(200, call); got > 36 {
		t.Errorf("the call allocates %v times; the dialect is being copied or the arithmetic read again", got)
	}
}
