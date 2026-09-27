// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What this shell says about a `[` it will not read, which is read from the
// **word counts of the segments between the connectives** — see
// interp/testcountedsegments.go for the rule, which is the flag
// `Diagnostics.TestRefusalCountsTheWordsBetweenConnectives` turns on.
//
// Every row below is status 2 in this shell and was status 2 here before,
// which is what makes this wording rather than behavior: only the sentence
// and the word it names moved.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) under
// `-f -c`, `env -i PATH=/usr/bin:/bin LC_ALL=C`; `go version -m` on that
// binary says *not a Go executable*. Forty-five operand lists, of which
// twenty decided the rule and twenty more were **predicted** onto it before
// being run.
//
// The location moves with the sentence and it is one rule rather than two:
// the builtin is named in the location for `too many arguments` and for
// `unknown condition`, and left out of it for both `condition expected`
// sentences. `test` behaves the same under its own name, which is the row
// that says the name is the word that was typed.
func TestWhatARefusedBracketSaysIsReadFromTheSegmentsWordCounts(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, expr, want string }{
		// Two words. The first must be an operator, and a word spelled like
		// one that this shell has not got is named.
		{"a plain word where an operator belongs", "a b", "zsh:1: parse error: condition expected: a"},
		{"a binary operator with nothing behind it", "foo -eq", "zsh:1: parse error: condition expected: foo"},
		{"a word spelled like an operator", "-q a", "zsh:[:1: unknown condition: -q"},

		// Three words. The operator is looked for at word 2 — unless word 1
		// is a known unary, which beats the binary reading and leaves word 3
		// over.
		{"a plain word where a binary operator belongs", "a b c", "zsh:1: condition expected: b"},
		{"an operator-spelled word in the binary place", "a -q b", "zsh:[:1: unknown condition: -q"},
		{"word 1 being a unary beats the binary form", "-n foo scrimble", "zsh:[:1: too many arguments"},
		{"and the same with a file test", "-e a b", "zsh:[:1: too many arguments"},

		// Four words and up. A primary off the front leaves the rest over;
		// with no primary the complaint names word 1.
		{"a primary and a word left over", "foo = bar baz", "zsh:[:1: too many arguments"},
		{"two primaries with no connective", "-n a -x b", "zsh:[:1: too many arguments"},
		{"a binary triple and two words over", "a = b c d", "zsh:[:1: too many arguments"},
		{"four bare words name the first", "a b c d", "zsh:1: condition expected: a"},
		{"and so do five", "a b c d e", "zsh:1: condition expected: a"},
		{"and six", "a b c d e f", "zsh:1: condition expected: a"},

		// The connectives, which is where the count is taken from: the
		// leftmost segment that does not read is what gets complained about,
		// and the two-word sentence holds inside a segment as it does at the
		// top level.
		{"a two-word segment on the right", "a -o b c", "zsh:1: parse error: condition expected: b"},
		{"a two-word segment on the left", "a b -a c", "zsh:1: parse error: condition expected: a"},
		{"a good segment then a two-word one", "-n a -a b c", "zsh:1: parse error: condition expected: b"},
		{"a triple then a two-word segment", "a = b -a c d", "zsh:1: parse error: condition expected: c"},
		{"a three-word segment names word 2", "-n a -o b c d", "zsh:1: condition expected: c"},
		{"and the leftmost failing segment wins", "a b -a c d -o e f", "zsh:1: parse error: condition expected: a"},

		// **The pair that makes it a count.** Same shape, one word longer,
		// and the word named moves from word 2 of a three-word segment to
		// word 1 of a four-word one. A reader that stopped where the words
		// stopped making sense would name the same word in both.
		{"a trailing connective leaves a three-word segment", "a b c -a", "zsh:1: condition expected: b"},
		{"and one word more makes it a four-word one", "a b c d -o", "zsh:1: condition expected: a"},

		// A leading `!` is off before the words are counted.
		{"a negated two-word shape", "! a b", "zsh:1: parse error: condition expected: a"},
		{"a negated three-word shape", "! a b c", "zsh:1: condition expected: b"},
		{"a negated primary with a word over", "! -n a b", "zsh:[:1: too many arguments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "[ "+tc.expr+" ]\n")
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("[ %s ] said %q, want %q", tc.expr, got, tc.want)
			}
			if st != 2 {
				t.Errorf("[ %s ] status %d, want 2", tc.expr, st)
			}
		})
	}
}

// The same rule under the other name the builtin is called by, which is what
// says the location carries the word that was typed rather than a fixed one.
func TestTheSegmentRuleReadsTestUnderItsOwnName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ expr, want string }{
		{"-n foo scrimble", "zsh:test:1: too many arguments"},
		{"a b", "zsh:1: parse error: condition expected: a"},
		{"a b c d", "zsh:1: condition expected: a"},
	} {
		out, st := runZsh(t, dir, "test "+tc.expr+"\n")
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("test %s said %q, want %q", tc.expr, got, tc.want)
		}
		if st != 2 {
			t.Errorf("test %s status %d, want 2", tc.expr, st)
		}
	}
}

// The controls, and they are two different kinds.
//
// The first is an expression this shell **reads**: the rule is about a
// refusal, so a list that is an expression must still be one, and a reader
// that had started refusing lists would pass every row above.
//
// The second is a refusal of *evaluation* rather than of arity — an operand a
// numeric comparison could not read, an operator with nothing behind it.
// The segment rule finds nothing wrong with the shape of those and they keep
// the sentence the ordinary reading made, which is how one column's wording
// is added without taking the rest of its `test` with it.
func TestTheSegmentRuleLeavesReadableListsAndEvaluationFailuresAlone(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a unary test", "[ -n a ]; printf 'st=%s' \"$?\"", "st=0", 0},
		{"a binary test", "[ a = a ]; printf 'st=%s' \"$?\"", "st=0", 0},
		{"a false one", "[ a = b ]; printf 'st=%s' \"$?\"", "st=1", 0},
		{"two primaries joined", "[ -n a -a b = b ]; printf 'st=%s' \"$?\"", "st=0", 0},
		{"three of them", "[ a -a b -a c ]; printf 'st=%s' \"$?\"", "st=0", 0},
		{"a connective the file test is not", "[ -a f ]", "zsh:[:1: too many arguments", 2},
		{"a trailing connective is the left side alone", "[ x -a ]; printf 'st=%s' \"$?\"", "st=1", 0},
		{"a numeric comparison of a word", "[ 1 -eq a ]", "zsh:[:1: integer expression expected: a", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src+"\n")
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s said %q, want %q", tc.src, got, tc.want)
			}
			if st != tc.status {
				t.Errorf("%s status %d, want %d", tc.src, st, tc.status)
			}
		})
	}
}
