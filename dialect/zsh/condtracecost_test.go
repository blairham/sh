// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"runtime"
	"testing"
)

// condTraceBytes is what one call of fn allocates, in bytes, against a runner
// holding a 16 KB value `x` (and its copy `y`) that a trace would quote.
func condTraceBytes(t *testing.T, defs, fn string) uint64 {
	t.Helper()
	call := localCostRunner(t, `x=${(l:16384::a b\'c:)}; y=$x`+"\n"+defs, fn)
	call()
	const n = 30
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range n {
		call()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / n
}

// A condition renders its operands for the trace only when it is traced.
// Every `[[ ]]` quoted its operands for an xtrace line before asking whether
// there would be one, so a comparison of a long value paid for a quoted copy
// nobody printed — measured at about 6 ms of an interactive start on a real
// configuration (#6363).
//
// The same condition is run in two functions, one with xtrace on, and the
// untraced one has to save the quoting: for a 16 KB operand holding spaces
// and an apostrophe that measures 140–290 KB a call. Before the fix the
// untraced call saved only what writing the line costs, 32–74 KB, because it
// rendered the operands all the same; the bound sits between the two.
func TestAnUntracedConditionDoesNotRenderItsOperands(t *testing.T) {
	for _, cond := range []string{`[[ -n $x ]]`, `[[ $x == $y ]]`, `[[ $x < $y ]] || :`} {
		defs := "traced() { setopt localoptions xtrace; " + cond + "; }\nplain() { setopt localoptions noxtrace; " + cond + "; }"
		traced, plain := condTraceBytes(t, defs, "traced 2>/dev/null"), condTraceBytes(t, defs, "plain")
		if plain+96<<10 > traced {
			t.Errorf("%s allocates %d bytes a call untraced and %d traced; untraced should save the quoting, at least 96 KB",
				cond, plain, traced)
		}
	}
}
