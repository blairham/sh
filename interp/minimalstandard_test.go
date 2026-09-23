// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The closed form is the same generator as stepping one draw at a time.
//
// Which is the only thing worth asserting about it here: what the numbers *mean*
// is a fact about a shell and is asserted in the dialect that answers for that
// shell, against the real binary. This file asks whether square-and-multiply
// drifts from the recurrence it stands in for — and a loop is the definition, so
// the two are compared rather than a table being written out.
func TestTheClosedFormAgreesWithSteppingOneDrawAtATime(t *testing.T) {
	step := func(seed, drawn uint64) uint64 {
		state := seed
		for range drawn {
			if state == 0 {
				state = minimalStandardRestart
			}
			state = state * minimalStandardMultiplier % minimalStandardModulus
		}
		return state
	}
	for _, seed := range []uint64{
		0, 1, 2, 3, 4, 42, 32767, 32768, 123459876,
		// The two a reduction can leave standing on the modulus: neither is
		// zero, and the recurrence takes both to zero in one step. They are
		// what the rule about a draw rather than about a seed is for.
		minimalStandardModulus, 2 * minimalStandardModulus,
		minimalStandardModulus - 1, 1 << 31, 1<<32 - 1,
	} {
		for drawn := uint64(0); drawn <= 64; drawn++ {
			if got, want := MinimalStandardState(seed, drawn), step(seed, drawn); got != want {
				t.Fatalf("seed %d after %d draws: closed form %d, stepping %d",
					seed, drawn, got, want)
			}
		}
		// And far enough out that the exponent has more than one bit set in
		// several places, which is where a square-and-multiply goes wrong.
		for _, drawn := range []uint64{999, 1000, 1023, 1024, 4095, 100000} {
			if got, want := MinimalStandardState(seed, drawn), step(seed, drawn); got != want {
				t.Fatalf("seed %d after %d draws: closed form %d, stepping %d",
					seed, drawn, got, want)
			}
		}
	}
}

// The state is never zero twice running, and never leaves the residues.
//
// Zero is the recurrence's fixed point, so a generator that reached it would
// answer the same number for the rest of the run. The restart is what prevents
// that, and this is the property it exists for rather than a restatement of the
// constant.
func TestTheStateNeverSettlesOnZero(t *testing.T) {
	for _, seed := range []uint64{0, minimalStandardModulus, 2 * minimalStandardModulus} {
		zeros := 0
		for drawn := uint64(1); drawn <= 200; drawn++ {
			state := MinimalStandardState(seed, drawn)
			if state >= minimalStandardModulus {
				t.Fatalf("seed %d after %d draws: state %d is not a residue", seed, drawn, state)
			}
			if state == 0 {
				zeros++
				if drawn > 1 {
					t.Errorf("seed %d: state is zero at draw %d, past the first", seed, drawn)
				}
			}
		}
		if zeros > 1 {
			t.Errorf("seed %d: state was zero %d times in 200 draws", seed, zeros)
		}
	}
}
