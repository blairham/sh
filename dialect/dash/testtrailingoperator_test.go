// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// `test` and `[` with a trailing operator, which this shell answers two ways
// and the panel mostly refuses.
//
// Measured 2026-09-18 on dash 0.5.12 as macOS 26 ships it, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` (#2917).
//
// One caveat about the reference, recorded because it is why these rows are
// asserted here and not through the suite: that binary **crashes** on
// `test x -a` when its standard error is redirected — `dash -c 'test x -a
// 2>/dev/null'` is a SIGSEGV — while `[ x -a ]` and the unredirected form are
// fine. The measurement above was taken through a command substitution, which
// survives it.

// A connective with nothing behind it is the connective still, over a right
// operand that is missing and therefore false. Silent, and the status is the
// whole answer: `-a` is 1 over a true left side where `-o` is 0.
func TestATrailingConnectiveIsAnsweredRatherThanRefused(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
		why  string
	}{
		{`[ x -a ]`, 1, "true and a missing right operand"},
		{`[ x -o ]`, 0, "true or one"},
		{`[ "" -a ]`, 1, "false and one"},
		{`[ "" -o ]`, 1, "false or one, which leaves it false"},
		{`[ -z a -a ]`, 1, "three words, the first an operator"},
		{`[ a = b -a ]`, 1, "three that are a comparison"},
		{`[ 1 -eq 1 -a ]`, 1, "four, past the count rules"},
		{`[ 1 -eq 1 -o ]`, 0, "and the other connective there"},
		{`[ x -a x -a ]`, 1, "two connectives, the last one bare"},
		{`[ \( x \) -a ]`, 1, "behind a group"},
		// The count rules still win, which is the control: three words are
		// the both-set guard over two strings here as everywhere.
		{`[ a -a -a ]`, 0, "three words are two strings and a connective"},
		{`[ -a ]`, 0, "and one word is a string"},
	} {
		out, st := runDashWith(t, tc.src+"\n", nil)
		if st != tc.want {
			t.Errorf("%s = %d, want %d — %s", tc.src, st, tc.want, tc.why)
		}
		if out != "" {
			t.Errorf("%s said %q, and this shell is silent about it", tc.src, out)
		}
	}
}

// A binary operator this shell has, standing where its right operand belonged,
// names **itself**. bash names the word in front and ksh93 names nothing, so
// this is the wording rather than the reading.
func TestATrailingBinaryOperatorNamesItself(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[ a = ]`, "[: =: argument expected"},
		{`[ a != ]`, "[: !=: argument expected"},
		{`[ a -eq ]`, "[: -eq: argument expected"},
		{`[ a -ne ]`, "[: -ne: argument expected"},
		{`[ a -ot ]`, "[: -ot: argument expected"},
		{`[ a -ef ]`, "[: -ef: argument expected"},
		// The set is the operators this shell has, which is what makes it a
		// reading and not a list: both string-order operators are in it and
		// `==`, which this shell has not got, falls back to the complaint
		// about the word in front.
		{`[ a \< ]`, "[: <: argument expected"},
		{`[ a \> ]`, "[: >: argument expected"},
		{`[ a == ]`, "[: a: unexpected operator"},
		// And `test` is the same builtin under another name.
		{`test a -eq`, "test: -eq: argument expected"},
	} {
		out, st := runDashWith(t, tc.src+"\n", nil)
		if st != 2 {
			t.Errorf("%s: status %d, want 2", tc.src, st)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want it to hold %q", tc.src, out, tc.want)
		}
	}
}
