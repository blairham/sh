// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestTheTruthOfAValueUnderOneIsNotItsTruncation is an end-to-end row rather
// than a suite case because no tier of share/suite can hold it.
//
// A ksh/ case has to be one no other reference matches and zsh matches this
// one; a core/ or ext/ case wants every reference to agree and the five
// columns without floats cannot be asked at all. Measured 2026-09-16 with each
// probe a script file under `env -i`:
//
//	                    bash 5.3   bash-as-sh   bash 3.2   zsh 5.9.2   ksh93u+   dash   ash
//	(( 0.5 ))              1           1           1          0           0       127   127
//	(( -0.5 ))             1           1           1          0           0       127   127
//	(( 0.0 ))              1           1           1          1           1       127   127
//	let 0.5                1           1           1          0           0       127   127
//
// bash's 1 is a refusal — it has no float to read — and dash and BusyBox ash
// have no `((` command and no `let`. So the two columns that can answer both
// call a value between zero and one true, and the status here took its answer
// from an *integer* evaluation, which truncates because an array subscript and
// an integer attribute have to. Truncation and the truth agree for every value
// of one or more, which is why `(( 1.5 ))` was already right and nothing had
// noticed.
//
// The operators inside an expression were never wrong — those ask whether the
// value is zero — so the rows below keep one of each beside the status, and a
// fix that reached only the operators would leave the first four failing.
func TestTheTruthOfAValueUnderOneIsNotItsTruncation(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a half is true", `(( 0.5 )); echo "$?"`, "0"},
		{"a negative half is true", `(( -0.5 )); echo "$?"`, "0"},
		{"a float zero is false", `(( 0.0 )); echo "$?"`, "1"},
		{"a sum that rounds to a half", `(( 0.4 + 0.1 )); echo "$?"`, "0"},
		{"let says the same", `let 0.5; echo "$?"`, "0"},
		{"let on a float zero", `let 0.0; echo "$?"`, "1"},
		{"and only the last of let decides", `let 1 0.5; echo "$?"`, "0"},
		{"an integer zero is still false", `(( 0 )); echo "$?"`, "1"},
		{"and one is still true", `(( 1 )); echo "$?"`, "0"},
		{"the negation was already right", `echo "$(( !0.5 ))"`, "0"},
		{"and the conditional", `echo "$(( 0.5 ? 7 : 9 ))"`, "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src+"\n")
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// TestANumeralPastTheWordIsTheDouble is the half of
// Semantics.ArithValuesAreCarriedInAFloat that interp's own batch cannot ask:
// the numeral reader takes the *dialect's* word for whether this shell has
// floats at all, and that harness is a shell with no dialect.
//
// Measured against AT&T ksh93u+ 2012-08-01 on 2026-09-16. The other three
// columns each read such a numeral differently again and none of those
// readings exists here yet — see #3202.
func TestANumeralPastTheWordIsTheDouble(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo "$(( 10000000000000000000 ))"`, "1e+19"},
		{`echo "$(( 18446744073709551616 ))"`, "1.84467440737096e+19"},
		{`echo "$(( 99999999999999999999999 ))"`, "1e+23"},
		{`echo "$(( 0xffffffffffffffffff ))"`, "4.72236648286965e+21"},
	} {
		out, st := answersRun(t, tc.src+"\n")
		if st != 0 || strings.TrimSpace(out) != tc.want {
			t.Errorf("%s: status %d, got %q, want %q", tc.src, st, strings.TrimSpace(out), tc.want)
		}
	}
}

// TestAHexadecimalFloatIsRead pins syntax.Dialect.ArithHexFloat end to end,
// which the suite file cannot do alone: two of its rows are refusals, and a
// refusal in a suite case ends the script.
//
// Measured 2026-09-16 against AT&T ksh93u+ 2012-08-01. Each row fails under a
// reader written to the decimal float's rules instead.
func TestAHexadecimalFloatIsRead(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"0x1p4", "16"},
		{"0x1P4", "16"},
		{"0x1p-1", "0.5"},
		{"0x1P-2", "0.25"},
		{"0x1p+2", "4"},
		{"0xffp0", "255"},
		{"0x1.8p1", "3"},
		{"0x1.8", "1.5"},
		{"0x1.p1", "2"},
		{"0x1.8p", "1.5"},
		{"0x0p0", "0"},
		{"-0x1p4", "-16"},
		// `e` is a hexadecimal digit, so these stay integers.
		{"0x1e5", "485"},
		{"0x1E5", "485"},
		{"0xff", "255"},
		// The exponent's digits may be missing where the letter is not.
		{"0x1p", "1"},
		{"0x1p+", "1"},
		{"0x1p-", "1"},
		// And the numeral still ends where an operator begins.
		{"0x1+1", "2"},
		{"0x1-1", "0"},
		{"0xf-1", "14"},
		{"0xff+1", "256"},
		// The decimal spelling is untouched by any of it.
		{"1e5", "100000"},
		{"1e-1", "0.1"},
	} {
		out, st := answersRun(t, `echo "$(( `+tc.src+` ))"`+"\n")
		if st != 0 || strings.TrimSpace(out) != tc.want {
			t.Errorf("$(( %s )): status %d, got %q, want %q", tc.src, st, strings.TrimSpace(out), tc.want)
		}
	}
}

// TestAHexadecimalFloatWantsAMantissaDigit is the other half: what ksh93
// refuses, which is about a missing digit and not a missing exponent.
func TestAHexadecimalFloatWantsAMantissaDigit(t *testing.T) {
	for _, src := range []string{"0x.8p1", "0xp4", "16#1p4", "0x1p1p1"} {
		out, st := answersRun(t, `echo "$(( `+src+` ))"`+"\n")
		if st == 0 {
			t.Errorf("$(( %s )): status 0 and output %q, want a refusal", src, out)
		}
	}
}
