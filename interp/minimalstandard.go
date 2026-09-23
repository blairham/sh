// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The generator three shells in the panel draw `RANDOM` from.
//
// Measured rather than known: `docs/spec/random.md` records the oracle runs and
// the method, and the short version is that `RANDOM=1` answers 16807 in two of
// them, which is one multiplication of the seed with its top bits taken off.
// The recurrence below is what the numbers say, confirmed on seeds chosen after
// the fit was made.
//
// It is in the substrate because **a recurrence names no shell.** What each
// dialect answers is the pair either side of it — how the value a script
// assigned becomes a state, and which bits of the state come back out as a
// number — and those are what differ: one recurrence, three sequences. See
// `dialect/bash/random.go` and its two siblings, and
// Runner.SetSeededRandoms for the seam.
const (
	// minimalStandardMultiplier and minimalStandardModulus are the Lehmer
	// generator's two constants. 16807 is a primitive root of 2^31-1, so the
	// state walks every nonzero residue before it repeats.
	minimalStandardMultiplier = 16807
	minimalStandardModulus    = 1<<31 - 1
	// minimalStandardRestart is the state a zero is replaced by, and all three
	// shells use the same one. Zero is the recurrence's fixed point, so no
	// implementation can let the state sit there.
	minimalStandardRestart = 123459876
)

// MinimalStandardState is the state the generator holds after `drawn` draws
// from `seed`, where `seed` is what the dialect's own reduction made of the
// value a script assigned.
//
// A closed form rather than a loop, and that is not an optimization for its own
// sake: the seam it serves hands over the seed and the draw count rather than a
// running state, so that the whole of a seeded generator is two integers a
// subshell carries away by value (#2827). Advancing one step at a time from the
// seed on every draw would make a script's *n*-th read cost *n*.
//
// One rule about zero, and it is the same rule in all three shells: **a state
// of exactly zero at the start of a draw is replaced by the restart before the
// multiplication.** That covers both the shell whose reduction lands in
// `1`..`2^31-2`, so a zero can only arrive at seed time, and the shell whose
// does not — zsh's `RANDOM=-2` is twice the modulus, answers `0` on the first
// draw and its zero-seed sequence from the second, which a rule written only
// about the seed would get wrong.
func MinimalStandardState(seed, drawn uint64) uint64 {
	if drawn == 0 {
		// Before any draw, which is the state a seed *is*: the substitution
		// happens at the start of a draw, so with no draws nothing has
		// happened to it and it is not yet a residue either.
		return seed
	}
	state := seed % minimalStandardModulus
	if seed != 0 && state == 0 {
		// A seed the recurrence sees as zero without being zero. The first
		// draw answers what a zero state answers, and the restart is what the
		// second draws from.
		if drawn == 1 {
			return 0
		}
		drawn--
		state = 0
	}
	if state == 0 {
		state = minimalStandardRestart
	}
	return mulMod(state, powMod(minimalStandardMultiplier, drawn))
}

// powMod is the multiplier raised to a draw count, modulo the generator's
// modulus: square and multiply, so a script's thousandth draw costs ten
// multiplications rather than a thousand.
func powMod(base, exp uint64) uint64 {
	result := uint64(1)
	base %= minimalStandardModulus
	for exp > 0 {
		if exp&1 == 1 {
			result = mulMod(result, base)
		}
		base = mulMod(base, base)
		exp >>= 1
	}
	return result
}

// mulMod multiplies two residues.
//
// Both are below 2^31, so the product is below 2^62 and a 64-bit multiply is
// exact — no splitting is needed, which is the only reason this is a function
// at all rather than an expression written out four times.
func mulMod(a, b uint64) uint64 {
	return a * b % minimalStandardModulus
}
