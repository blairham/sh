// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An underscore inside a numeral is a digit separator here, so the numeral is
// worth what it would be without it.
//
// The parser's own test says how far the numeral reaches; this one says what
// it comes to, which is the half a lexer change alone does not get right.
// Measured against zsh 5.9.2, 2026-09-13 — every row is one the shell answers
// and the other six refuse, so none of them is a POSIX fact wearing this
// shell's name. #2223.
func TestADigitSeparatorIsSkippedInsideANumeral(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		// The row that decides it. `1_` is worth 1 under a separator reading
		// and under a discarded-byte reading alike, so it cannot.
		{"1_0", "10"},
		{"1_0_0", "100"},
		{"1__0", "10"},
		{"1_2_3", "123"},
		{"1_", "1"},
		{"10_", "10"},
		{"0_", "0"},
		{"-1_0", "-10"},
		// Removed before the base is applied, in every spelling of a numeral.
		{"0x1_f", "31"},
		{"0b1_0", "2"},
		{"2#1_0", "2"},
		{"16#f_f", "255"},
		{"1_0#5", "5"},
		// One may stand where the first digit would.
		{"0x_1", "1"},
		{"2#_10", "2"},
		// And in a float, in the fraction and in the exponent alike.
		{"1_0.5", "10.5"},
		{"1.5_0", "1.5"},
		{"1._5", "1.5"},
		{"1_.5", "1.5"},
		{".5_0", "0.5"},
		{"1e1_0", "10000000000."},
		{"1_e2", "100."},
		{"1e_2", "100."},
	} {
		if got := arithLine(t, tc.expr); got != tc.want+"\n" {
			t.Errorf("$((%s)) = %q, want %q", tc.expr, got, tc.want+"\n")
		}
	}
}

// It is a rule about reading a numeral and not about reading a script, so it
// reaches a value no parser ever saw.
//
// Its own test because an implementation can pass every row above by changing
// only the lexer and still refuse this one: the text `1_0` here was never in
// the program.
func TestADigitSeparatorReachesAStoredValue(t *testing.T) {
	out, st := answersRun(t, `x=1_0; echo "$(( x ))"; y=0x1_f; echo "$(( y ))"`)
	if want := "10\n31\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// A numeral begins with a digit, so a leading underscore is a name and not a
// separator standing in front of one.
//
// Measured: `$(( _1 ))` is 0 there, an unset name being zero, where a reading
// that skipped the byte first would make it 1.
func TestALeadingUnderscoreIsAName(t *testing.T) {
	out, st := answersRun(t, `echo "$(( _1 ))"; _1=7; echo "$(( _1 ))"`)
	if want := "0\n7\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The separator hides the byte it stands on and no other, so the numeral
// still ends where its base runs out.
func TestASeparatorDoesNotHideTheLettersAfterIt(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"1_abc", "bad math expression: operator expected at `abc'"},
		{"0x1_g", "bad math expression: operator expected at `g'"},
	} {
		if got := arithLine(t, tc.expr); got != arithLoc+tc.want+"\n" {
			t.Errorf("$((%s)) = %q, want %q", tc.expr, got, arithLoc+tc.want+"\n")
		}
	}
}
