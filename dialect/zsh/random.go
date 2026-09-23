// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The sequence this shell's `RANDOM` answers once a script has seeded it.
//
// The recurrence is the substrate's — interp.MinimalStandardState, measured and
// argued in `docs/spec/random.md`. What is here is the two answers that are this
// shell's own.
//
// Measured 2026-09-22 on `/opt/homebrew/bin/zsh` 5.9.2 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`: seeds 0 through 200, a two-hundred draw
// run from 12345, eleven out-of-range and negative seeds, and 100 fresh
// pseudo-random seeds chosen *after* the fit. No exceptions.
func registerRandoms(r *interp.Runner) {
	r.SetSeededRandoms(func(seed, drawn uint64) int {
		return randomFold(interp.MinimalStandardState(randomSeedState(seed), drawn))
	})
}

// randomSeedState is what the value a script assigned becomes: a 32-bit wrap
// and nothing else.
//
// **Not reduced modulo the generator's modulus**, which is the measurement that
// separates this shell from bash and is the reason the substrate's zero rule is
// written about a draw rather than about a seed. `RANDOM=-2` is 2^32-2, which is
// twice the modulus: the state is not zero, the first draw multiplies it to zero
// and answers `0`, and only the second draws from the restart — so `RANDOM=-2;
// echo "$RANDOM $RANDOM"` is `0 20034` here where bash answers 20814 first for
// the same arithmetic value.
func randomSeedState(seed uint64) uint64 {
	return uint64(uint32(seed))
}

// randomFold is which bits of the state come back out: the low fifteen, and
// nothing folded onto them.
//
// At seed 4 the state is 67228 and this answers 1692 where bash's fold answers
// 1693. That one row is the whole measured difference between the two
// sequences.
func randomFold(state uint64) int {
	return int(state & 0x7fff)
}
