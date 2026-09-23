// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/interp"

// The sequence this shell's `RANDOM` answers once a script has seeded it.
//
// The recurrence is the substrate's — interp.MinimalStandardState, measured and
// argued in `docs/spec/random.md`. What is here is the two answers that are this
// shell's own, and this shell differs from the other two in **both** of them.
//
// Measured 2026-09-22 on `/bin/ksh` (Version AJM 93u+ 2012-08-01) under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`: seeds 0 through 400, a three-hundred
// draw run from 12345, seventeen out-of-range and negative seeds, and 100 fresh
// pseudo-random seeds chosen *after* the fit. No exceptions.
//
// A long run has to be taken with `while` rather than `for i in $(seq …)` here:
// this shell reseeds `RANDOM` across the fork a command substitution makes, so
// the substitution disturbs the sequence being measured — which first read as
// the model failing on the very first draw.
func registerRandoms(r *interp.Runner) {
	r.SetSeededRandoms(func(seed, drawn uint64) int {
		return randomFold(interp.MinimalStandardState(randomSeedState(seed), drawn))
	})
}

// randomSeedState is what the value a script assigned becomes: **fifteen bits**,
// which is the narrowest reduction in the panel and was pinned by the seeds that
// overflow it rather than by ordinary ones.
//
// `RANDOM=-1` and `RANDOM=2147483647` answer alike — both are state 32767 — and
// `RANDOM=2147483648` answers what `RANDOM=0` answers. Neither pair agrees under
// a 31-bit or 32-bit reduction, and the first pair is what ruled out reducing
// modulo the generator's modulus: this shell's state can be 32767 and answer a
// number, where a modulus reduction of 2^31-1 would make it zero.
func randomSeedState(seed uint64) uint64 {
	return seed & 0x7fff
}

// randomFold is which bits of the state come back out: three shifted off the
// bottom, then fifteen.
//
// Visible at the smallest seeds, which is where it was read off: `RANDOM=1`
// answers 2100 here where bash and zsh both answer 16807, and 2100 is 16807
// shifted right by three. `RANDOM=8` answers 16807, the state 134456 shifted the
// same way.
func randomFold(state uint64) int {
	return int((state >> 3) & 0x7fff)
}
