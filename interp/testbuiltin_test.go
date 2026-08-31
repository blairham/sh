// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `test` is defined by argument count before grammar, so these are organized
// by count rather than by operator. Every rule here is unanimous across the
// panel; only the wording of a malformed expression diverges, and that lives
// in dialect/.

func testStatus(t *testing.T, dir, src string) int {
	t.Helper()
	_, st := lookRun(t, dir, dir, src)
	return st
}

// TestArgumentCountDecidesTheMeaning is the rule a grammar alone would not
// predict, and the reason `test` surprises people.
func TestArgumentCountDecidesTheMeaning(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		// None: false, and not an error.
		{`test`, 1},
		// One: a string, however it is spelled. `-f` here is a non-empty
		// string, not an operator missing its operand.
		{`test x`, 0},
		{`test ""`, 1},
		{`test -f`, 0},
		// Two: an operator, or a negation.
		{`test -n x`, 0},
		{`test -z ""`, 0},
		{`test ! ""`, 0},
		{`test ! x`, 1},
		// Three: a comparison, a negated unary, or a parenthesised string.
		{`test a = a`, 0},
		{`test a = b`, 1},
		{`test ! -n ""`, 0},
		{`test \( x \)`, 0},
		// Four: a negated comparison, or a parenthesised unary.
		{`test ! a = b`, 0},
		{`test \( -n x \)`, 0},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestEqualsComparesStringsAndNeverPatterns is the sharpest difference from
// `[[ ]]`, where the same right operand is a pattern and the answer flips.
func TestEqualsComparesStringsAndNeverPatterns(t *testing.T) {
	dir := t.TempDir()
	if got := testStatus(t, dir, `test abc = "a*"`); got != 1 {
		t.Errorf("test abc = a* gave %d, want 1: `=` compares text", got)
	}
	if got := testStatus(t, dir, `test "a*" = "a*"`); got != 0 {
		t.Errorf("the same text on both sides should compare equal, got %d", got)
	}
	// Byte-for-byte, which a fold or a locale-aware compare would not be. A
	// mutation making `=` case-insensitive passed until this was here.
	if got := testStatus(t, dir, `test A = a`); got != 1 {
		t.Errorf("test A = a gave %d, want 1: the comparison is exact", got)
	}
}

// TestNumericOperatorsCompareNumbers — the word-spelled operators are numeric
// and `=` is textual, which is the trap `[[ ]]` has too.
func TestNumericOperatorsCompareNumbers(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test 10 -gt 9`, 0},
		{`test 10 -lt 9`, 1},
		{`test 2 -eq 2`, 0},
		{`test 2 -ne 2`, 1},
		{`test 1 -le 1`, 0},
		{`test 1 -ge 2`, 1},
		// Textually, "10" sorts before "9".
		{`test 10 = 9`, 1},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

func TestFileOperators(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "full"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "exe"), []byte("x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test -e full`, 0},
		{`test -e nope`, 1},
		{`test -f full`, 0},
		{`test -f adir`, 1},
		{`test -d adir`, 0},
		{`test -d full`, 1},
		{`test -s full`, 0},
		{`test -s empty`, 1},
		{`test -x exe`, 0},
		{`test -x full`, 1},
		{`test -r full`, 0},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestAndBindsTighterThanOr is the precedence, and the grammar only takes over
// past four arguments.
func TestAndBindsTighterThanOr(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test a = a -a b = b`, 0},
		{`test a = a -a b = c`, 1},
		{`test a = b -o c = c`, 0},
		// (false and true) or true, rather than false and (true or true).
		{`test a = b -a b = b -o c = c`, 0},
		{`test \( a = b -o c = c \) -a d = d`, 0},
		{`test \( a = b -o c = d \) -a d = d`, 1},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestTheQuotingTrap is why `[ ]` needs quotes where `[[ ]]` does not. An
// unquoted empty expansion is not an empty argument — it is no argument, so
// the count changes and with it the meaning.
func TestTheQuotingTrap(t *testing.T) {
	dir := t.TempDir()
	if got := testStatus(t, dir, `u=; test -n $u`); got != 0 {
		t.Errorf("unquoted: %d, want 0 — `-n` is left as a lone non-empty string", got)
	}
	if got := testStatus(t, dir, `u=; test -n "$u"`); got != 1 {
		t.Errorf("quoted: %d, want 1 — a real -n test on an empty operand", got)
	}
}

// TestBracketWantsItsBracket — `[` is a command and `]` is an argument, so
// nothing but the builtin checks for it.
func TestBracketWantsItsBracket(t *testing.T) {
	dir := t.TempDir()
	if got := testStatus(t, dir, `[ x ]`); got != 0 {
		t.Errorf("[ x ] = %d, want 0", got)
	}
	if got := testStatus(t, dir, `[ x`); got != 2 {
		t.Errorf("[ without ] = %d, want 2", got)
	}
	// And `]` is not special to `test`, which is why this is malformed rather
	// than a second way of writing the same thing.
	if got := testStatus(t, dir, `test x ]`); got != 2 {
		t.Errorf("test x ] = %d, want 2", got)
	}
}

// TestMalformedIsTwoAndFalseIsOne is the distinction a single non-zero would
// lose. A script that branches on `$?` can tell "this is not an expression"
// from "the expression is false".
func TestMalformedIsTwoAndFalseIsOne(t *testing.T) {
	dir := t.TempDir()
	if got := testStatus(t, dir, `test a = b`); got != 1 {
		t.Errorf("a false expression = %d, want 1", got)
	}
	for _, src := range []string{
		`test -Q x`,
		`test a b c`,
		`test x -eq 1`,
	} {
		if got := testStatus(t, dir, src); got != 2 {
			t.Errorf("%s = %d, want 2", src, got)
		}
	}
}

// TestTheDialectWordsAMalformedExpression covers the four wordings, which are
// the only part of `test` the panel disagrees about.
func TestTheDialectWordsAMalformedExpression(t *testing.T) {
	dir := t.TempDir()
	out, _ := lookRun(t, dir, dir, `test -Q x`)
	if !strings.Contains(out, "-Q") {
		t.Errorf("output %q should name the operator it did not recognize", out)
	}
	out, _ = lookRun(t, dir, dir, `test a b c`)
	if !strings.Contains(out, "b") {
		t.Errorf("output %q should name the word where an operator belonged", out)
	}
}
