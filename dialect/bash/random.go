// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/interp"

// The sequence this shell's `RANDOM` answers once a script has seeded it.
//
// The recurrence is the substrate's, because a recurrence names no shell —
// interp.MinimalStandardState, and `docs/spec/random.md` records how it was
// measured. What is here is the two answers that are this shell's own, and they
// are the whole of why three shells with one recurrence draw three different
// sequences.
//
// Measured 2026-09-22 on `/opt/homebrew/bin/bash` 5.3.20 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`: seeds 0 through 300, a three-thousand
// draw run from 12345, seventeen out-of-range and negative seeds, and 124 fresh
// pseudo-random seeds chosen *after* the fit was made. No exceptions.
//
// Seed 4 is the row that separates this shell from zsh and is worth keeping in
// mind when reading the two functions below: the state is 67228, which has a bit
// above 16. This shell folds it back in and answers 1693 where zsh masks it off
// and answers 1692. Seed 1 cannot tell them apart, because 16807 has nothing
// above bit 15 to fold.
func registerRandoms(r *interp.Runner) {
	r.SetSeededRandoms(func(seed, drawn uint64) int {
		return randomFold(interp.MinimalStandardState(randomSeedState(seed), drawn))
	})
}

// randomSeedState is what the value a script assigned becomes.
//
// Two reductions and the order matters. The 32-bit wrap is first and is visible
// on its own: `RANDOM=4294967296` is the zero seed here, where reducing modulo
// the generator's own modulus first would make 2^32 the seed 2. Then the modulus,
// which is why `RANDOM=2147483647` answers what `RANDOM=0` answers and
// `RANDOM=2147483648` answers what `RANDOM=1` does.
//
// So the state handed on is always in `0`..`2^31-2`, and a zero can only arrive
// at seed time — which is why this shell never shows the other half of the
// substrate's zero rule, and zsh does.
func randomSeedState(seed uint64) uint64 {
	return uint64(uint32(seed)) % randomModulus
}

// randomFold is which bits of the state come back out as a number: the top half
// folded onto the bottom with exclusive-or, and fifteen bits of that.
//
// Not a mask, which is the single measurement this rests on and the one that
// separates this shell from zsh. At seed 4 the state is 67228: masking gives
// 1692 and folding gives 1693, and bash answers 1693.
func randomFold(state uint64) int {
	return int(((state >> 16) ^ (state & 0xffff)) & 0x7fff)
}

// randomModulus is the generator's modulus, named here because the seed
// reduction needs it and the substrate keeps its copy unexported — the
// recurrence is the substrate's business and the reduction is this shell's.
const randomModulus = 1<<31 - 1
