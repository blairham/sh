// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A tie's separator is **one byte** — the third operand's first — and an
// operand with no bytes in it gives the byte NUL.
//
// Three readings agree on every ordinary separator and part only at the
// edges, which is why this file drives the edges: "the whole operand", "the
// first character" and "the first byte" all say `:` for `:`. The operand
// `ab` parts the first from the other two, and a multibyte character parts
// the second from the third.
//
// The empty operand is the row the listing rests on. It is not a separate
// rule: a byte taken from a string with no bytes in it is the zero byte, so
// `typeset -T A a ''` and `typeset -T A a $'\0'` are the same declaration —
// and `''` is then the only word whose first byte is that separator, which
// is why it is the spelling a listing writes and why the listing round-trips
// (#4515).
//
// Tests name axes and letters, never shells.

// withTiesAndNulBytes is withTies plus the one axis a NUL written inside
// `$'…'` needs, so a probe can say which byte the separator is rather than
// only how many bytes the join came to.
func withTiesAndNulBytes(s *Semantics) {
	withTies(s)
	s.DollarSingleNul = DollarSingleNulIsAByte
}

func TestATiesSeparatorIsTheOperandsFirstByte(t *testing.T) {
	for _, tc := range []struct{ name, operand, want string }{
		// One byte of a two-byte operand: the join is three bytes, not four.
		{"a two-character operand", `ab`, "3"},
		// The same first byte, a different rest, and the same answer — which
		// is what says the rule is keyed on the byte and not on the string.
		{"the same first byte alone", `a`, "3"},
		// A multibyte character contributes its lead byte alone, so the join
		// is three bytes rather than four.
		{"a multibyte character", `é`, "3"},
		// The control: an ordinary one-byte separator, where all three
		// readings agree.
		{"one byte", `:`, "3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "typeset -T A a '" + tc.operand + "'\na=(p q)\nprintf '%s' \"${#A}\""
			out, errs, st := declRun(t, src, withTies, Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("%q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// The empty operand, in both directions and in the listing — the three
// places a separator is read.
func TestAnEmptyTieSeparatorIsTheNulByte(t *testing.T) {
	const src = `typeset -T A a ''
a=(p q)
printf '1 len=%s ' "${#A}"
[[ $A == $'p\0q' ]] && printf 'joined-on-NUL ' || printf 'joined-on-something-else '
typeset -T B b ''
B=$'x\0y\0z'
printf '2 n=%s [%s] ' "${#b[@]}" "${b[*]}"
typeset -p A`
	want := "1 len=3 joined-on-NUL 2 n=3 [x y z] typeset -T A a=( p q ) ''\n"
	out, errs, st := declRun(t, src, withTiesAndNulBytes, Diagnostics{})
	if out != want || st != 0 || errs != "" {
		t.Errorf("%q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// Written as an escape rather than as an empty word, which is the same
// separator by the rule above and has to list back the same way: a listing
// that wrote the byte as an escape would not round-trip, because no word
// whose first byte is NUL can be written any other way.
func TestANulTieSeparatorListsAsTheEmptyWord(t *testing.T) {
	const src = `typeset -T A a $'\0'
a=(p q)
printf '%s|' "${#A}"
typeset -p A`
	want := "3|typeset -T A a=( p q ) ''\n"
	out, errs, st := declRun(t, src, withTiesAndNulBytes, Diagnostics{})
	if out != want || st != 0 || errs != "" {
		t.Errorf("%q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The default is still the default, and a listing still leaves it off — the
// control that says the change above did not turn "no operand" into the NUL
// byte as well.
func TestATieWithNoSeparatorOperandKeepsTheDefault(t *testing.T) {
	const src = `typeset -T A a
A=x:y
printf '%s|' "${a[*]}"
typeset -p A`
	want := "x y|typeset -T A a=( x y )\n"
	out, errs, st := declRun(t, src, withTiesAndNulBytes, Diagnostics{})
	if out != want || st != 0 || errs != "" {
		t.Errorf("%q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
