// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"testing"
	"time"
)

// Comparing a value with a quoted copy of itself is linear in its length
// under extendedglob, whatever operator characters it holds.
//
// A quoted operand reaches the matcher as a pattern with a backslash in front
// of each operator character, and the matcher reads a pattern a piece at a
// time. At every piece it asked whether what was left split on a `~`, and a
// value holding one `~` anywhere — escaped, so never a split — made each of
// those questions a walk to the end. powerlevel10k compares two signatures of
// about 3 KB this way at every start, and the two comparisons cost about 10
// ms where zsh 5.9.2 takes 40 µs (#6363).
//
// A ratio of two sizes rather than a number, so the machine's speed and load
// cancel; the best of five with the collector held off, for the reasons
// interp/indexedarray_test.go gives. Linear is a ratio near 4 and quadratic
// one near 16.
func TestAQuotedComparisonIsLinearInTheValue(t *testing.T) {
	if testing.Short() {
		t.Skip("times two runs")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	best := func(n int) time.Duration {
		// One `~`, then n items that are each a piece: `(a|b)`, `x`.
		defs := fmt.Sprintf(`setopt extendedglob; x="~${(l:%d::(a|b)x:)}"; y=$x`, 6*n)
		call := quotedCompareRunner(t, defs)
		b := time.Duration(1<<63 - 1)
		for range 5 {
			runtime.GC()
			start := time.Now()
			call()
			b = min(b, time.Since(start))
		}
		return b
	}
	const n = 500
	small, large := best(n), best(4*n)
	if ratio := float64(large) / float64(small); ratio > 9 {
		t.Fatalf("comparing %d items took %v and %d took %v: ratio %.2f, want about 4 (linear)",
			n, small, 4*n, large, ratio)
	}
}

// quotedCompareRunner runs defs and returns the comparison, failing the test
// if the value does not equal its own quoted copy.
func quotedCompareRunner(t *testing.T, defs string) func() {
	t.Helper()
	call := localCostRunner(t, defs+"\nfail() { exit 3; }", `[[ $x == "$y" ]] || fail`)
	return call
}
