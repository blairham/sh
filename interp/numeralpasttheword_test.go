// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An integer numeral larger than the machine word, which this shell refused
// and every column on the panel answers — see
// Semantics.ArithNumeralPastTheWord.
//
// Every row is measured, and every row below discriminates: each one is
// answered differently by at least two of the three readings, so a table that
// held only the first numeral would pass under two answers it is not.

func wordRun(t *testing.T, src string, p NumeralPastTheWord, dg Diagnostics) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArithNumeralPastTheWord = p
		sem.ArithStoredNumeralPastTheWordIsRefused = No
		r.Semantics, r.Diagnostics = &sem, &dg
	})
}

func TestANumeralPastTheWordTakesTheDialectsReading(t *testing.T) {
	for _, tc := range []struct {
		name    string
		snippet string
		reading NumeralPastTheWord
		want    string
	}{
		// The unsigned word goes round, and it is arithmetic modulo 2^64 and
		// not a stop at the top of it: 2^64-1 is -1 and 2^64 is 0, where a
		// reader that saturated unsigned would answer -1 twice.
		{"the wrap", `echo $((10000000000000000000))`, NumeralPastTheWordWraps, "-8446744073709551616"},
		{"the wrap at the unsigned maximum", `echo $((18446744073709551615))`, NumeralPastTheWordWraps, "-1"},
		{"the wrap one past it", `echo $((18446744073709551616))`, NumeralPastTheWordWraps, "0"},
		{"the wrap several times round", `echo $((99999999999999999999999))`, NumeralPastTheWordWraps, "200376420520689663"},
		{"the wrap in hexadecimal", `echo $((0xffffffffffffffffff))`, NumeralPastTheWordWraps, "-1"},

		// Saturation answers the same number for every one of them, which is
		// what makes it distinguishable from the wrap on any row at all.
		{"the clamp", `echo $((10000000000000000000))`, NumeralPastTheWordSaturates, "9223372036854775807"},
		{"the clamp several times round", `echo $((99999999999999999999999))`, NumeralPastTheWordSaturates, "9223372036854775807"},
		{"the clamp in hexadecimal", `echo $((0xffffffffffffffffff))`, NumeralPastTheWordSaturates, "9223372036854775807"},

		// Truncation, and these are the rows a fit-while-it-fits reader gets
		// wrong. The first two it agrees on; the rest it does not.
		{"a truncation that fits", `echo $((10000000000000000000))`, NumeralPastTheWordKeepsTheDigitsThatFit, "1000000000000000000"},
		{"a truncation at the unsigned maximum", `echo $((18446744073709551615))`, NumeralPastTheWordKeepsTheDigitsThatFit, "1844674407370955161"},
		// Past the *unsigned* word, so reading stopped during the scan and
		// what it had stands — negative, and no digit was put back.
		{"a truncation that keeps a value past the signed word", `echo $((99999999999999999999999))`, NumeralPastTheWordKeepsTheDigitsThatFit, "-8446744073709551617"},
		// The digits ran out with the value past the signed word, and the
		// last one comes off the *wrapped* number: 5e19 modulo 2^64 divided
		// by ten, which is not a prefix of the numeral at all.
		{"a truncation that divides what it has", `echo $((50000000000000000000))`, NumeralPastTheWordKeepsTheDigitsThatFit, "1310651185258089676"},
		// And these two say it is the *wrapped* value being divided rather
		// than the numeral's true prefix — the row above cannot, its last
		// digit being a zero, and a reader that went back to the prefix
		// answers 5288635833802085649 and 8880021581086201913 here. Both
		// measured against zsh 5.9.2.
		{"a divided back-off after a wrap", `echo $((237353799075116372652))`, NumeralPastTheWordKeepsTheDigitsThatFit, "1599287019060175326"},
		{"a divided back-off one word past", `echo $((88800215810862019133))`, NumeralPastTheWordKeepsTheDigitsThatFit, "1501323951602381266"},
		// Read whole and silently: every digit made the running value larger
		// even though the word went round on the way.
		{"a numeral past the unsigned word that still rises", `echo $((22222222222222222222))`, NumeralPastTheWordKeepsTheDigitsThatFit, "3775478148512670606"},
		{"a truncation in hexadecimal", `echo $((0xffffffffffffffffff))`, NumeralPastTheWordKeepsTheDigitsThatFit, "1152921504606846975"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := wordRun(t, tc.snippet, tc.reading, Diagnostics{})
			if st != 0 || strings.TrimSpace(out) != tc.want {
				t.Errorf("out=%q st=%d, want %q at status 0", out, st, tc.want)
			}
		})
	}
}

// The count in the diagnostic is a second measurement and not a restatement of
// the value: three of these rows keep the same value under the reading that
// stops at the last digit to fit, and differ only here.
func TestATruncatedNumeralSaysHowFarItRead(t *testing.T) {
	dg := Diagnostics{ArithNumberTruncated: "number truncated after %[1]d digits: %[2]s"}
	for _, tc := range []struct{ name, snippet, want string }{
		{"the digits and the tail", `echo $(( 9999999999999999999 ))`, "number truncated after 18 digits: 9999999999999999999 "},
		{"a wrap during the scan", `echo $(( 99999999999999999999 ))`, "number truncated after 19 digits: 99999999999999999999 "},
		{"more digits than the word could ever hold", `echo $(( 12345678901234567890123 ))`, "number truncated after 22 digits: 12345678901234567890123 "},
		{"a hexadecimal run, counted without its prefix", `echo $(( 0xfffffffffffffffff ))`, "number truncated after 16 digits: fffffffffffffffff "},
		{"the tail is the rest of the expression", `echo $((9999999999999999999 + 1))`, "number truncated after 18 digits: 9999999999999999999 + 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := wordRun(t, tc.snippet, NumeralPastTheWordKeepsTheDigitsThatFit, dg)
			if st != 0 || !strings.Contains(out, tc.want) {
				t.Errorf("out=%q st=%d, want %q in it at status 0", out, st, tc.want)
			}
		})
	}
	// Silence is the other half: a dialect with no sentence for it answers
	// the numeral and says nothing.
	out, st := wordRun(t, `echo $(( 9999999999999999999 ))`, NumeralPastTheWordKeepsTheDigitsThatFit, Diagnostics{})
	if st != 0 || strings.TrimSpace(out) != "999999999999999999" {
		t.Errorf("out=%q st=%d, want the value alone", out, st)
	}
}

// A numeral that stood in a variable, which one column refuses where it
// answers the same digits written down.
func TestAStoredNumeralPastTheWordIsRefusedWhereTheDialectSaysSo(t *testing.T) {
	src := "n=10000000000000000000\necho $(( n ))\necho \"st=$?\"\n"
	refuse := func(r *Runner) {
		sem := CoreSemantics()
		sem.ArithNumeralPastTheWord = NumeralPastTheWordSaturates
		sem.ArithStoredNumeralPastTheWordIsRefused = Yes
		dg := Diagnostics{InvalidNumber: "Illegal number: %s"}
		r.Semantics, r.Diagnostics = &sem, &dg
	}
	out, _ := run(t, src, refuse)
	if !strings.Contains(out, "Illegal number: 10000000000000000000") {
		t.Errorf("out=%q, want the stored numeral refused", out)
	}
	// And the same digits written down are answered by the same dialect,
	// which is the whole of what the axis says.
	out, st := run(t, "echo $(( 10000000000000000000 ))\n", refuse)
	if st != 0 || strings.TrimSpace(out) != "9223372036854775807" {
		t.Errorf("out=%q st=%d, want the written numeral saturated", out, st)
	}
}

// An axis nothing answered refuses, naming itself, rather than falling back to
// one column's reading — and says it once.
func TestANumeralPastTheWordWithNoReadingRefusesOnce(t *testing.T) {
	out, _ := run(t, "echo $(( 10000000000000000000 ))\necho \"st=$?\"\n", func(r *Runner) {
		sem := CoreSemantics()
		r.Semantics = &sem
	})
	if !strings.Contains(out, "an integer numeral past the machine word") {
		t.Errorf("out=%q, want the axis named", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want the refusal at status 2", out)
	}
	if n := strings.Count(out, "the shells disagree here"); n != 1 {
		t.Errorf("out=%q says it %d times, want once", out, n)
	}
}
