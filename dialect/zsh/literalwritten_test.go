// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// An array literal does not print its elements' source text unless a refusal
// asks for it. F-Sy-H's highlighter builds its reply one literal at a time —
// `reply+=("$__start $__end ${_fsh_styles[$__style]}")` — and each element was
// printed back to source on the way, for a sentence that only a keyed literal
// can ever say (#6423). See interp/arrayliteral.go's literalElem.written.
//
// Allocations rather than time, so the row is deterministic.
func TestALiteralDoesNotPrintItsElements(t *testing.T) {
	const defs = "typeset -A st; st=(k fg=red); a=1 b=2 s=k; typeset -a reply\n" +
		`f() { reply=(); reply+=("$a $b ${st[$s]}"); reply+=("$b $a ${st[$s]}" x $a); }` + "\n"
	call := localCostRunner(t, defs, "f")
	call()
	// 40 on the change that set this, and 65 before it.
	if got := testing.AllocsPerRun(200, call); got > 50 {
		t.Errorf("the call allocates %v times; the literal is printing its elements again", got)
	}
}
