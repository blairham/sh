// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "math/bits"

// DialectOptions is a dialect's own option state: one bit for each option
// the dialect numbers, and the one valued name an option namespace has that
// is not a bit, the emulation mode.
//
// It exists because of what saving and restoring cost. One dialect turns its
// whole option table around on every function call — zsh's `emulate -L zsh`,
// which nearly every function in a zsh plugin opens with, and LOCAL_OPTIONS
// at every return — and when this state lived in the variable store, as an
// association of names and a scalar under names no script can reach, each
// save copied a map, each restore built one, and each question about a
// single option hashed a name to find a variable and then hashed it again to
// find the key. Measured on the maintainer's real configuration (2026-10-05),
// a function that opens with `emulate -L zsh` cost about five times what
// real zsh spends on the same call. Held here, a save is a copy of a small
// value and a question is a shift and a mask (#6100).
//
// **By value on the runner, so a subshell keeps its own** — clone copies the
// struct and the copy is the subshell's, which is the isolation the variable
// store gave the same state by deep-copying its tables. `(setopt auto_cd)`
// stays in the subshell for that reason and no other.
//
// The numbering is the dialect's: the substrate stores and copies the bits
// and attaches no meaning to any of them. A dialect with no options of this
// kind leaves it zero and pays nothing for it.
type DialectOptions struct {
	bits [dialectOptionWords]uint64
	// Mode is the emulation in force, or empty for the dialect's own. A
	// string, so a copy is a header and not the text.
	Mode string
}

// dialectOptionWords is how many 64-bit words the bits take. Four holds 256
// options, which is room above the largest table in the tree (zsh's, 185).
// An index past it is out of range and panics, which is why a dialect's own
// test holds its table against DialectOptionCapacity.
const dialectOptionWords = 4

// DialectOptionCapacity is how many options DialectOptions can number. A
// dialect's own test checks its table against it, so that an option added
// past the end is a failing test rather than a bit written into nothing.
const DialectOptionCapacity = dialectOptionWords * 64

// Has reports whether option i is set.
func (o *DialectOptions) Has(i int) bool {
	return o.bits[i>>6]&(1<<(uint(i)&63)) != 0
}

// Set moves option i.
func (o *DialectOptions) Set(i int, on bool) {
	if on {
		o.bits[i>>6] |= 1 << (uint(i) & 63)
	} else {
		o.bits[i>>6] &^= 1 << (uint(i) & 63)
	}
}

// None reports whether no option is set.
func (o *DialectOptions) None() bool {
	for _, w := range o.bits {
		if w != 0 {
			return false
		}
	}
	return true
}

// Each calls f with every option that is set, in ascending order.
func (o *DialectOptions) Each(f func(i int)) {
	for w, word := range o.bits {
		for word != 0 {
			b := bits.TrailingZeros64(word)
			f(w*64 + b)
			word &^= 1 << uint(b)
		}
	}
}

// Merge replaces the options mask selects with the same options of from,
// leaving every other option as it is. It is how a whole group of options is
// put back to a precomputed state in one step: an emulation resetting the
// names it resets, for one.
func (o *DialectOptions) Merge(from, mask *DialectOptions) {
	for w := range o.bits {
		o.bits[w] = o.bits[w]&^mask.bits[w] | from.bits[w]&mask.bits[w]
	}
}

// SameBits reports whether two states hold the same options, ignoring Mode.
func (o *DialectOptions) SameBits(other *DialectOptions) bool {
	return o.bits == other.bits
}
