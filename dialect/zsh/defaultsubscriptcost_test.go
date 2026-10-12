// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// `${A[$k]-}` reads its subscript once, as `${A[$k]}` does.
//
// The `-` asks whether the element is set, and that test asked whether the
// subscript spelled the whole array by expanding the subscript's word again.
// Expanding a word starts a span, and a span lets go of what the expansion
// around it was holding, so the expansion then read the subscript a third
// time: `${A[$k]-}` cost half as much again as `${A[$k]}` here, where zsh
// 5.9.2 takes the same time for both (#6423). A plugin writes it wherever it
// reads a table it does not know the keys of — F-Sy-H's lifecycle check runs
// `${functions[$name]-}` over forty names after every keystroke.
//
// Allocations rather than time, so the row is deterministic.
func TestADefaultOnAnElementReadsTheSubscriptOnce(t *testing.T) {
	const defs = "typeset -A A; A=(k v); k=k\nautoload -Uz stubfn\nn=stubfn\n"
	for _, tc := range []struct{ plain, dashed string }{
		{"x=${A[$k]}", "x=${A[$k]-}"},
		{"x=${functions[$n]}", "x=${functions[$n]-}"},
	} {
		plain, dashed := localCostRunner(t, defs, tc.plain), localCostRunner(t, defs, tc.dashed)
		plain()
		dashed()
		base, got := testing.AllocsPerRun(200, plain), testing.AllocsPerRun(200, dashed)
		// 7 against 9 and 24 against 26 on the change that set this; 12
		// against 9 and 46 against 26 before it.
		if got > base {
			t.Errorf("%s allocates %v times and %s %v; the subscript is being read again", tc.dashed, got, tc.plain, base)
		}
	}
}
