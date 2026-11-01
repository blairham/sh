// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// The sizes and the bound, in items: the value is one `~` and then that many
// copies of `(a|b)x`, each of which is six pieces of a pattern. quotedBound
// sits between the ratio a linear comparison comes to, 6 to 8, and the 30 and
// more of one that walks to the end of the value at each item.
const (
	quotedShort          = 250
	quotedLong           = 8 * quotedShort
	quotedSamples        = 32
	quotedControlSamples = 8
	quotedBound          = 16
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
// That comparison no longer reaches the matcher: a pattern that is nothing but
// an escaped literal is compared as a string (#6436). So it is one row, and
// the other puts the same quoted value in front of a `?`, which the matcher
// has to read piece by piece. Without that row both walks #6416 removed could
// come back unseen: put back either one and the first row's ratio does not
// move, while the second's goes past the bound.
//
// A ratio of two sizes rather than a number, so the machine's speed cancels:
// the least of quotedSamples comparisons at quotedShort items and at
// quotedLong, the two taking turns, with the collector held off while one is
// timed. It used to be the best of five at one size and then the best of five
// at the other, the shape that made interp's
// TestAppendingToAnIndexedArrayIsLinear fail a busy runner with nothing wrong
// (#7020, #7022): timed one after the other, a stretch of load lands on one
// size only, and taking the best of five cannot undo a slowdown that hits all
// five. Taking turns puts it on both, and each comparison is short, under
// 0.5 ms (4 ms under the race detector), so a preemption is no more a thing of
// one size than the other. Load only ever adds time, so to fail now every
// comparison at the larger size has to be slowed past the bound while one at
// the smaller is not.
//
// **The control is the comparison followed by a walk to the end of the value
// from each item**, the shape #6416 removed, and the check fails unless the
// measurement sees it grow past the bound: one that cannot tell the two apart
// passes everything.
func TestAQuotedComparisonIsLinearInTheValue(t *testing.T) {
	if testing.Short() {
		t.Skip("times comparisons")
	}
	t.Run("compared as a string", func(t *testing.T) {
		checkAQuotedComparisonIsLinear(t, `[[ $x == "$y" ]]`)
	})
	t.Run("read by the matcher", func(t *testing.T) {
		checkAQuotedComparisonIsLinear(t, `[[ $x == "${y%x}"? ]]`)
	})
}

func checkAQuotedComparisonIsLinear(t *testing.T, cond string) {
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	type size struct {
		call  func()
		value string
	}
	at := func(n int) size {
		defs := fmt.Sprintf(`setopt extendedglob; x="~${(l:%d::(a|b)x:)}"; y=$x`, 6*n)
		return size{quotedCompareRunner(t, defs, cond), "~" + strings.Repeat("(a|b)x", n)}
	}
	short, long := at(quotedShort), at(quotedLong)
	// compare times one comparison at sz, and then then, which is handed the
	// value.
	compare := func(sz size, then func(string)) time.Duration {
		runtime.GC()
		start := time.Now()
		sz.call()
		then(sz.value)
		return time.Since(start)
	}
	// least is the least of samples comparisons at each size, the two taking
	// turns.
	least := func(samples int, then func(string)) (onShort, onLong time.Duration, ratio float64) {
		onShort, onLong = math.MaxInt64, math.MaxInt64
		for range samples {
			onShort = min(onShort, compare(short, then))
			onLong = min(onLong, compare(long, then))
		}
		return onShort, onLong, float64(onLong) / float64(max(onShort, 1))
	}
	nothing := func(string) {}
	operators := 0
	// walkFromEachItem reads to the end of v from the start of each item,
	// pairing backslashes as a pattern's scans do, and never stops early.
	walkFromEachItem := func(v string) {
		for i := 1; i < len(v); i += len("(a|b)x") {
			for j := i; j < len(v); j++ {
				switch v[j] {
				case '\\':
					j++
				case '~', '(', '|', ')':
					operators++
				}
			}
		}
	}
	compare(short, nothing) // untimed: the first of each
	compare(long, nothing)
	s, l, ratio := least(quotedControlSamples, walkFromEachItem)
	t.Logf("control, walking from each item: %v at %d items, %v at %d, ratio %.2f (%d operators passed)",
		s, quotedShort, l, quotedLong, ratio, operators)
	if ratio <= quotedBound {
		t.Fatalf("the control, a comparison followed by a walk to the end from each item, took %v at %d items and "+
			"%v at %d: ratio %.2f, so the measurement cannot see that walk", s, quotedShort, l, quotedLong, ratio)
	}
	s, l, ratio = least(quotedSamples, nothing)
	t.Logf("%s: %v at %d items, %v at %d, ratio %.2f", cond, s, quotedShort, l, quotedLong, ratio)
	if ratio > quotedBound {
		t.Fatalf("%s took %v at %d items and %v at %d: ratio %.2f, want about %d (linear)",
			cond, s, quotedShort, l, quotedLong, ratio, quotedLong/quotedShort)
	}
}

// quotedCompareRunner runs defs and returns cond, failing the test if it does
// not hold.
func quotedCompareRunner(t *testing.T, defs, cond string) func() {
	t.Helper()
	return localCostRunner(t, defs+"\nfail() { exit 3; }", cond+" || fail")
}
