// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a refused `test` is worded from the word counts of the segments
// between the connectives, or from where the reading stopped — see
// Diagnostics.TestRefusalCountsTheWordsBetweenConnectives and
// interp/testcountedsegments.go, which carry the measurement.
//
// Asked here by the flag and never by a shell's name, and the wordings are
// plain enough that an assertion is about which sentence and which word
// rather than about anybody's phrasing.
func countsTheSegments(on bool) func(*Runner) {
	return func(r *Runner) {
		r.Diagnostics = &Diagnostics{
			TestUnaryExpected:  "%[1]s: unknown operator",
			TestBinaryExpected: "%[1]s: not an operator",
			// Its own string, so a row can say which of the two
			// operator's-place sentences was reached.
			TestTwoWordOperatorExpected:                 "two words: %[1]s",
			TestOperandExpected:                         "argument expected",
			TestTooManyArguments:                        "left over",
			TestIntegerExpected:                         "%[1]s: integer expected",
			TestRefusalCountsTheWordsBetweenConnectives: on,
		}
		s := *r.Semantics
		s.TestStringOrder = TestStringOrderBoth
		r.Semantics = &s
	}
}

// saidBy drops the speaker this Runner writes in front of a diagnostic. The
// location split — which of these sentences carries the builtin's name — is a
// dialect's question and is asserted where a dialect's wordings live.
func saidBy(out string) string {
	return strings.TrimPrefix(strings.TrimSpace(out), "sh: ")
}

func TestARefusalReadFromTheSegmentsWordCounts(t *testing.T) {
	for _, tc := range []struct {
		src              string
		counted, stopped string
		why              string
	}{
		// Two words take a sentence of their own where the flag is on, and
		// the ordinary operator's-place one where it is off.
		{`test a b`, "two words: a", "a: unknown operator", "word 1 is not an operator"},
		{`test -q a`, "-q: unknown operator", "-q: unknown operator", "and is spelled like one"},

		// Three words: the operator is looked for at word 2, unless word 1
		// is a known unary — which is the row where the two readings part.
		{`test a b c`, "b: not an operator", "b: not an operator", "word 2 is not an operator"},
		{`test a -q b`, "-q: unknown operator", "-q: not an operator", "and is spelled like one"},
		{`test -n a b`, "left over", "a: not an operator", "word 1 wins and word 3 is over"},

		// Four and up: a primary off the front leaves the rest over, and
		// with no primary the complaint names word 1.
		{`test -n a -x b`, "left over", "left over", "two primaries, no connective"},
		{`test a = b c d`, "left over", "left over", "a triple and two words over"},
		{`test a b c d`, "a: not an operator", "left over", "no primary at all"},
		{`test a b c d e f`, "a: not an operator", "left over", "and however many more"},

		// The connectives are where the counts are taken from: the leftmost
		// segment that does not read is what is complained about.
		{`test a -o b c`, "two words: b", "left over", "a two-word segment on the right"},
		{`test a b -a c`, "two words: a", "left over", "and on the left"},
		{`test -n a -o b c d`, "c: not an operator", "left over", "a three-word segment"},

		// **The pair that makes it a count.** The same shape one word
		// longer names the other word, which a reader that stopped where the
		// words stopped making sense cannot do.
		{`test a b c -a`, "b: not an operator", "left over", "three words in front of it"},
		{`test a b c d -o`, "a: not an operator", "left over", "four words in front of it"},

		// A leading `!` is off before the words are counted.
		{`test ! a b`, "two words: a", "a: unknown operator", "a negated two-word shape"},
		{`test ! a b c`, "b: not an operator", "b: not an operator", "a negated three-word one"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			for _, r := range []struct {
				on   bool
				want string
			}{{true, tc.counted}, {false, tc.stopped}} {
				out, status := run(t, tc.src+"\n", countsTheSegments(r.on))
				if got := saidBy(out); got != r.want {
					t.Errorf("on=%v: %s said %q, want %q — %s", r.on, tc.src, got, r.want, tc.why)
				}
				if status != 2 {
					t.Errorf("on=%v: %s status %d, want 2 — this is a wording, not a verdict",
						r.on, tc.src, status)
				}
			}
		})
	}
}

// The controls, and they are two kinds.
//
// The first is that the flag words a **refusal** and does not make one: a
// list that reads is still read, under either answer. A reader that had begun
// refusing lists would pass every row above.
//
// The second is that a failure of *evaluation* rather than of arity keeps the
// sentence the ordinary reading made, because the segment rule finds nothing
// wrong with the shape of it.
func TestCountingTheSegmentsWordsRefusesNothingOfItsOwn(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{`test -n a; printf 'st=%s' "$?"`, "st=0", 0},
		{`test a = a; printf 'st=%s' "$?"`, "st=0", 0},
		{`test a = b; printf 'st=%s' "$?"`, "st=1", 0},
		{`test -n a -a b = b; printf 'st=%s' "$?"`, "st=0", 0},
		{`test a -a b -a c; printf 'st=%s' "$?"`, "st=0", 0},
		{`test 1 -eq a`, "a: integer expected", 2},
	} {
		t.Run(tc.src, func(t *testing.T) {
			for _, on := range []bool{true, false} {
				out, status := run(t, tc.src+"\n", countsTheSegments(on))
				if got := saidBy(out); got != tc.want {
					t.Errorf("on=%v: %s said %q, want %q", on, tc.src, got, tc.want)
				}
				if status != tc.status {
					t.Errorf("on=%v: %s status %d, want %d", on, tc.src, status, tc.status)
				}
			}
		})
	}
}

// And the sentence falls back where a dialect has not got one of its own,
// which is what keeps the two-word form where it was for every column that
// words the operator's places alike.
func TestTheTwoWordSentenceFallsBackToTheUnaryOne(t *testing.T) {
	out, _ := run(t, "test a b\n", func(r *Runner) {
		r.Diagnostics = &Diagnostics{
			TestUnaryExpected:                           "%[1]s: unknown operator",
			TestBinaryExpected:                          "%[1]s: not an operator",
			TestTooManyArguments:                        "left over",
			TestRefusalCountsTheWordsBetweenConnectives: true,
		}
	})
	if got, want := saidBy(out), "a: unknown operator"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
