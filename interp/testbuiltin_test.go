// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
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

// The operators past the three-word rules, each behind its own axis because
// each is a split rather than a correction — see the four fields on Semantics
// whose names begin `TestHas`, and TestStringOrderPolicy for the pair that
// needed an enum.

// TestTheUnaryFileExistsLetter is `-a` with two arguments, which is `-e`'s
// question where the argument count has already said the letter is an
// operator rather than the connective.
func TestTheUnaryFileExistsLetter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	yes := func(r *Runner) {
		s := *r.Semantics
		s.TestHasTheFileExistsLetter = Yes
		r.Semantics = &s
		r.Dir = dir
	}
	no := func(r *Runner) {
		s := *r.Semantics
		s.TestHasTheFileExistsLetter = No
		r.Semantics = &s
		r.Dir = dir
	}
	for _, tc := range []struct {
		name string
		set  func(*Runner)
		src  string
		want int
	}{
		{"the file is there", yes, `test -a f`, 0},
		{"the file is not", yes, `test -a nosuch`, 1},
		// The same letter inside the grammar, where it begins a primary as
		// well as joining two of them.
		{"a primary and a connective in one expression", yes, `test -a f -a -a f`, 0},
		{"and the connective still binds them", yes, `test -a f -a -a nosuch`, 1},
		// Without it the word is an operator this shell does not have, which
		// is 2 and not a false answer.
		{"refused where the shell has no such operator", no, `test -a f`, 2},
		// The connective is not the axis, and is unanimous: three words are
		// two strings joined however the letter is answered.
		{"three words are the connective either way", no, `test x -a y`, 0},
		{"and with an empty side", no, `test x -a ""`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st := run(t, tc.src, tc.set)
			if st != tc.want {
				t.Errorf("%s = %d, want %d", tc.src, st, tc.want)
			}
		})
	}
}

// TestTheShellOptionOperator is `-o name`: a `set -o` switch asked as an
// expression. A name the shell has never heard of is false rather than an
// error, which is measured in both shells that have the operator.
func TestTheShellOptionOperator(t *testing.T) {
	set := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.TestHasTheShellOptionOperator = a
			r.Semantics = &s
		}
	}
	for _, tc := range []struct {
		src  string
		a    Answer
		want int
	}{
		{`test -o errexit`, Yes, 1},
		{`set -e; test -o errexit`, Yes, 0},
		{`test -o nosuchopt`, Yes, 1},
		{`test -o errexit`, No, 2},
	} {
		_, st := run(t, tc.src, set(tc.a))
		if st != tc.want {
			t.Errorf("%s at %v = %d, want %d", tc.src, tc.a, st, tc.want)
		}
	}
}

// TestTheModifiedSinceReadOperator is `-N`: the modification time against the
// access time. The axis pins the operator and this pins the comparison, which
// is the half the panel agrees on — see the field's comment for why the
// boolean is not what a corpus row may grade.
func TestTheModifiedSinceReadOperator(t *testing.T) {
	dir := t.TempDir()
	written := filepath.Join(dir, "written")
	read := filepath.Join(dir, "read")
	for _, p := range []string{written, read} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Written after it was read, and read after it was written: the two
	// orders, spelled with explicit times so nothing depends on how fast the
	// test runs.
	early, late := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	if err := os.Chtimes(written, early, late); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(read, late, early); err != nil {
		t.Fatal(err)
	}
	set := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.TestHasTheModifiedSinceReadOperator = a
			r.Semantics = &s
			r.Dir = dir
		}
	}
	for _, tc := range []struct {
		src  string
		a    Answer
		want int
	}{
		{`test -N written`, Yes, 0},
		{`test -N read`, Yes, 1},
		// A file that is not there, and an operand that is not a path: false
		// rather than an error, measured.
		{`test -N nosuch`, Yes, 1},
		{`test -N ""`, Yes, 1},
		{`test -N written`, No, 2},
	} {
		_, st := run(t, tc.src, set(tc.a))
		if st != tc.want {
			t.Errorf("%s at %v = %d, want %d", tc.src, tc.a, st, tc.want)
		}
	}
}

// TestTheStringOrderOperators is the enum, and the third answer is why it is
// one: a shell can have `>` and refuse `<`.
func TestTheStringOrderOperators(t *testing.T) {
	set := func(p TestStringOrderPolicy) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.TestStringOrder = p
			r.Semantics = &s
		}
	}
	for _, tc := range []struct {
		src  string
		p    TestStringOrderPolicy
		want int
	}{
		// Byte order, and a string comparison rather than a numeric one.
		{`test a "<" b`, TestStringOrderBoth, 0},
		{`test b "<" a`, TestStringOrderBoth, 1},
		{`test b ">" a`, TestStringOrderBoth, 0},
		{`test A "<" a`, TestStringOrderBoth, 0},
		{`test 10 "<" 9`, TestStringOrderBoth, 0},
		// The greater one alone: the same two words are an answer under one
		// operator and not an expression under the other.
		{`test b ">" a`, TestStringOrderGreaterOnly, 0},
		{`test a "<" b`, TestStringOrderGreaterOnly, 2},
		{`test a "<" b`, TestStringOrderNeither, 2},
		{`test b ">" a`, TestStringOrderNeither, 2},
	} {
		_, st := run(t, tc.src, set(tc.p))
		if st != tc.want {
			t.Errorf("%s at %v = %d, want %d", tc.src, tc.p, st, tc.want)
		}
	}
}

// TestAMissingOrderOperatorIsNamedPastThreeWords is the same fault arriving
// by the other door. `<` is not spelled like a unary operator, so without a
// reader for it the left operand becomes a bare string, the expression
// parses, and the count is reported instead of the one token that was wrong —
// which is the #1290 shape, and which says nothing a reader can act on.
func TestAMissingOrderOperatorIsNamedPastThreeWords(t *testing.T) {
	out, st := run(t, `test -n x -a a "<" b`, func(r *Runner) {
		s := *r.Semantics
		s.TestStringOrder = TestStringOrderNeither
		r.Semantics = &s
		dg := Diagnostics{TestBinaryExpected: "%[2]s: %[1]s: unknown operator"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "<: unknown operator") {
		t.Errorf("out %q, want the operator named", out)
	}
	if strings.Contains(out, "too many arguments") {
		t.Errorf("out %q, want the count not reported over the token", out)
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
	// And where the shell has the operator, the same words are an answer.
	_, st = run(t, `test -n x -a a "<" b`, func(r *Runner) {
		s := *r.Semantics
		s.TestStringOrder = TestStringOrderBoth
		r.Semantics = &s
	})
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// TestAConnectiveWhereAUnaryBelongs is the wording split the two letters
// leave behind in a shell that has the connectives and not the file test:
// `-a` is a word zsh knows, so it is a string with a word left over, where a
// letter nothing has is an operator it has never heard of.
func TestAConnectiveWhereAUnaryBelongs(t *testing.T) {
	leftover := func(r *Runner) {
		s := *r.Semantics
		s.TestHasTheFileExistsLetter = No
		s.TestHasTheShellOptionOperator = No
		r.Semantics = &s
		dg := Diagnostics{
			TestConnectiveIsALeftoverWord: true,
			TestTooManyArguments:          "too many arguments",
			TestUnaryExpected:             "unknown condition: %[1]s",
		}
		r.Diagnostics = &dg
	}
	for _, src := range []string{`test -a f`, `test -o errexit`, `test -a f -a -a f`} {
		out, st := run(t, src, leftover)
		if !strings.Contains(out, "too many arguments") || st != 2 {
			t.Errorf("%s said %q at %d, want the leftover-word complaint", src, out, st)
		}
	}
	// And a letter the shell really has never heard of keeps the other one,
	// which is the pair that makes this a distinction rather than a rename.
	out, _ := run(t, `test -Q f`, leftover)
	if !strings.Contains(out, "unknown condition: -Q") {
		t.Errorf("said %q, want the unknown operator named", out)
	}
}
