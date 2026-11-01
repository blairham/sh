// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"testing"
)

// TestKeepOperandKeepsWhatItIsGiven runs keepOperand over every pattern of
// kept and dropped operands of a four-operand line and requires the list it
// builds to be exactly the kept ones, in order — and args itself never to be
// written, including by an append made to the list afterwards, which is what
// namesAfterARefusal does. The list is the front of args until an operand is
// dropped, so every pattern with a drop goes through the copy.
func TestKeepOperandKeepsWhatItIsGiven(t *testing.T) {
	t.Parallel()
	orig := []string{"a", "b", "c", "d"}
	for mask := range 1 << len(orig) {
		args := slices.Clone(orig)
		var rest, want []string
		for i := range args {
			if mask&(1<<i) != 0 {
				rest = keepOperand(rest, args, i)
				want = append(want, args[i])
			}
		}
		if !slices.Equal(rest, want) {
			t.Errorf("mask %04b: kept %q, want %q", mask, rest, want)
		}
		_ = append(rest, "appended")
		if !slices.Equal(args, orig) {
			t.Errorf("mask %04b: args became %q", mask, args)
		}
	}
}

// TestKeepOperandMakesNothingForAWholeLine is the allocation the alias saves:
// a line whose every operand is a name is passed on as it came.
func TestKeepOperandMakesNothingForAWholeLine(t *testing.T) {
	args := []string{"a=1", "b=2", "c"}
	allocs := testing.AllocsPerRun(100, func() {
		var rest []string
		for i := range args {
			rest = keepOperand(rest, args, i)
		}
		if len(rest) != len(args) {
			t.Fatal(rest)
		}
	})
	if allocs != 0 {
		t.Errorf("a line of names allocated %v times; want 0", allocs)
	}
}
