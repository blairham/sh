// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// #3212 gave this shell the C double ksh93 carries every arithmetic value in.
// It did not give it the *reader*, and eight rows diverged — every one of them
// a numeral past the machine word, parting along a line the double alone does
// not draw (#3257).
//
// **A numeral with an explicit radix is read in the unsigned word and wraps in
// it**, and becomes the double only when the *unsigned* word overflows. A
// plain decimal never takes that route and goes to the double as soon as it
// passes the *signed* maximum. So the same magnitude answers two ways
// depending on how it was spelt, which is the whole of the rule.
//
// Measured 2026-09-16 against /bin/ksh — AT&T 93u+ 2012-08-01, the build
// cmd/ksh targets and not ksh93u+m 1.0.8 — each probe a script file under
// `env -i` with standard input on /dev/null.
//
// End-to-end here rather than in interp's own batch for the reason
// TestANumeralPastTheWordIsTheDouble gives: the numeral reader takes the
// dialect's word for whether this shell has floats at all, and that harness is
// a shell with no dialect. And rather than in share/suite/ksh for a second
// reason — a ksh/ case must be one no other reference matches, and bash and
// BusyBox ash answer `-1` to the hexadecimal rows too, by a different road
// that stops at no unsigned edge. There is no tier that can hold a defect
// where two columns agree and the rule behind the agreement does not.
func TestARadixNumeralTakesTheUnsignedWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// Under the signed maximum nothing is in question, and the row is
		// here so that a reader which sent *every* hexadecimal numeral
		// through the unsigned route would still have to be right about this
		// one.
		{"the signed maximum", "0x7fffffffffffffff", "9223372036854775807"},

		// The three that part a signed read from an unsigned one. Each is
		// 1.84467440737096e+19 or 9.22337203685478e+18 under the double
		// alone, which is what this shell answered.
		{"the sign bit alone", "0x8000000000000000", "-9223372036854775808"},
		{"two short of the unsigned maximum", "0xfffffffffffffffe", "-2"},
		{"the unsigned maximum", "0xffffffffffffffff", "-1"},

		// One past the unsigned word, where the double takes over. The
		// fallback is `strtod`, so a hexadecimal numeral is the hexadecimal
		// float — 2^64 and not 2·10^16.
		{"one past the unsigned word", "0x10000000000000000", "1.84467440737096e+19"},
		{"well past it", "0x1ffffffffffffffff", "3.68934881474191e+19"},
		{"and further still", "0xfffffffffffffffff", "2.95147905179353e+20"},

		// The octal pair, which is what makes this a rule rather than a
		// width. 2^64-1 in octal is held exactly by the unsigned word and
		// reads as -1; 2^64 in octal overflows it, and the reader it falls
		// back to has never heard of octal — so the answer is the *decimal*
		// reading of the same digits, leading zero and all.
		{"the unsigned maximum in octal", "01777777777777777777777", "-1"},
		{"one past it in octal", "02000000000000000000000", "2e+21"},
		{"and a longer octal run", "017777777777777777777770", "1.77777777777778e+22"},

		// The same magnitudes written in decimal, which take no unsigned
		// route at all. These are the rows that make the ones above a fact
		// about the radix rather than about the value.
		{"the unsigned maximum in decimal", "18446744073709551615", "1.84467440737096e+19"},
		{"one past the unsigned word in decimal", "18446744073709551616", "1.84467440737096e+19"},

		// The wrapped value is an **integer**, so the integer operators do
		// integer work on it. A reader that handed back a float would answer
		// -0.333333333333333 to the division.
		{"a wrapped value added to", "0xffffffffffffffff + 0", "-1"},
		{"a wrapped value divided", "0xffffffffffffffff / 3", "0"},
		{"a wrapped value multiplied", "0xffffffffffffffff * 1", "-1"},

		// And it goes back through carriedInADouble like every other integer,
		// which is the row that says so: the unsigned word holds 2^63+1
		// exactly and the double this shell stores it in does not, so the
		// answer is one *below* the exact reading.
		{"a value the double cannot keep", "0x8000000000000001", "-9223372036854775808"},

		// The sign parseNum strips before the conversion, which the
		// conversion never gets to put back when it fails.
		{"a negated wrapped value", "-0xffffffffffffffff", "1"},

		// Untouched: a numeral under the word, in either spelling.
		{"an ordinary hexadecimal", "0xff", "255"},
		{"an ordinary octal", "0777", "511"},
		// And this shell's tolerance for a digit octal cannot use, which is
		// the condition the range failure used to be folded into.
		{"a digit octal cannot use", "08", "8"},
		{"and another", "09", "9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, `echo "$(( `+tc.src+` ))"`+"\n")
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("$(( %s )): status %d, got %q, want %q",
					tc.src, st, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// TestADoubleIsWrittenAsAnIntegerWhenTheCastRoundTrips is the other half of
// #3257, and it is not about a numeral at all.
//
// The shell whose every arithmetic value is a C double writes one as an
// **integer** whenever a saturating `(intmax_t)` cast of it converts back to
// the same double, and in floating notation when it does not. #3212 wrote that
// rule down and applied it to an integer *result* only, so a value that
// arrived as a float — a float literal, a division of two of them, a numeral
// past the signed word — was written in floating notation whatever the cast
// said.
//
// Measured 2026-09-16 against /bin/ksh, AT&T 93u+ 2012-08-01.
func TestADoubleIsWrittenAsAnIntegerWhenTheCastRoundTrips(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The numeral rows the issue names. 2^63 is not the maximum and the
		// cast saturates to one below it — and that number converts back to
		// 2^63, so the round trip holds and the value is written as an
		// integer.
		{"the numeral at two to the sixty-third", "9223372036854775808", "9223372036854775807"},
		{"and one past it", "9223372036854775809", "9223372036854775807"},

		// A float divided, with no integer anywhere in it. `1e19` is a float
		// literal in every column that has one, so this row cannot be about
		// the word at all.
		{"a float literal divided", "1e19/3", "3333333333333333504"},
		{"a numeral past the word divided", "18446744073709551616 / 3", "6148914691236516864"},
		{"and a decimal one", "10000000000000000000 / 3", "3333333333333333504"},

		// Where the cast does not round trip the floating notation stands,
		// which is the half a rule applied everywhere would have lost.
		{"a numeral whose cast does not round trip", "10000000000000000000", "1e+19"},
		{"the float literal itself", "1e19", "1e+19"},
		{"two to the sixty-fourth", "2**64", "1.84467440737096e+19"},

		// And under the word nothing changes, in either direction.
		{"a whole float", "1.5 + 1.5", "3"},
		{"a half", "0.5", "0.5"},
		{"a third", "1.0/3", "0.333333333333333"},
		{"a float quotient", "3.0/2", "1.5"},
		{"a large whole float", "2.5e10", "25000000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, `echo "$(( `+tc.src+` ))"`+"\n")
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("$(( %s )): status %d, got %q, want %q",
					tc.src, st, strings.TrimSpace(out), tc.want)
			}
		})
	}
}
