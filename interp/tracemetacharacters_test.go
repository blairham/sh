// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which words `set -x` quotes, which is a separate question from how it
// spells the quoting once something has decided — see Diagnostics.
// TraceMetacharacters and #2141.
//
// Named for the field rather than for the shells that picked each set; the
// presets' picks are asserted in dialect/. What is asserted here is that the
// *shape* carries the panel: a set that fires anywhere, a set that fires only
// at the front, and the two independent of each other.

// tracedWord runs `echo` on one already-quoted literal and gives back the
// traced line, with the prefix trimmed.
func tracedWord(t *testing.T, word string, meta TraceMetacharacters) string {
	t.Helper()
	src := "set -x; echo '" + word + "'"
	got := traceOf(t, src, permissive(), Diagnostics{
		TraceQuoting:        QuoteShell,
		TraceMetacharacters: meta,
	})
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(got), "+ echo "), "\n")
}

// TestTheAlwaysQuotedSetNeedsNoAnswer is the floor: with both fields empty a
// word still gets quoted for whitespace, a quote, or an operator that would
// reparse. A dialect that answers nothing here is not a dialect that quotes
// nothing.
func TestTheAlwaysQuotedSetNeedsNoAnswer(t *testing.T) {
	for _, w := range []string{"a b", `a$b`, "a|b", "a;b", "a(b"} {
		if got := tracedWord(t, w, TraceMetacharacters{}); got != "'"+w+"'" {
			t.Errorf("%q with no answer = %s, want it quoted", w, got)
		}
	}
	for _, w := range []string{"ab", "a-b", "a:b", "a/b"} {
		if got := tracedWord(t, w, TraceMetacharacters{}); got != w {
			t.Errorf("%q with no answer = %s, want it bare", w, got)
		}
	}
}

// TestTheAnywhereSetFiresWhereverItAppears is the half a character list alone
// could already express.
func TestTheAnywhereSetFiresWhereverItAppears(t *testing.T) {
	meta := TraceMetacharacters{Anywhere: "*~"}
	for _, w := range []string{"a*b", "*ab", "ab*", "a~b", "~ab", "ab~"} {
		if got := tracedWord(t, w, meta); got != "'"+w+"'" {
			t.Errorf("%q in the anywhere set = %s, want it quoted", w, got)
		}
	}
	for _, w := range []string{"a#b", "#ab"} {
		if got := tracedWord(t, w, meta); got != w {
			t.Errorf("%q outside both sets = %s, want it bare", w, got)
		}
	}
}

// TestTheLeadingSetFiresOnlyAtTheFront is the half that made the old single
// ContainsAny unable to express any of the three shells, whatever characters
// were put in it.
//
// The pair of rows is the whole point: the same character, quoted in one
// position and bare in the other, under one answer.
func TestTheLeadingSetFiresOnlyAtTheFront(t *testing.T) {
	meta := TraceMetacharacters{Leading: "~#"}
	for _, w := range []string{"~a", "#a", "~", "#"} {
		if got := tracedWord(t, w, meta); got != "'"+w+"'" {
			t.Errorf("%q at the front = %s, want it quoted", w, got)
		}
	}
	for _, w := range []string{"a~b", "a#b", "a~", "a#"} {
		if got := tracedWord(t, w, meta); got != w {
			t.Errorf("%q away from the front = %s, want it bare", w, got)
		}
	}
}

// TestTheTwoTraceSetsAreIndependent: a character in one is not in the other,
// which is what lets one answer say "`~` anywhere" and another "`~` only at
// the front" without either being a subset of the other.
func TestTheTwoTraceSetsAreIndependent(t *testing.T) {
	meta := TraceMetacharacters{Anywhere: "^", Leading: "="}
	rows := []struct {
		word string
		want string
	}{
		{"^ab", `'^ab'`},
		{"ab^", `'ab^'`},
		{"=ab", `'=ab'`},
		{"ab=", "ab="},
		{"a=b", "a=b"},
	}
	for _, row := range rows {
		if got := tracedWord(t, row.word, meta); got != row.want {
			t.Errorf("%q = %s, want %s", row.word, got, row.want)
		}
	}
}

// TestTraceQuotingAndTheSetsAreSeparateQuestions: the spelling and the
// decision do not move together. Two shells share QuoteShell and disagree
// about the set, and a shell with QuoteDollar has a set of its own — which is
// why they are two fields.
func TestTraceQuotingAndTheSetsAreSeparateQuestions(t *testing.T) {
	meta := TraceMetacharacters{Anywhere: "*"}
	// QuoteNever ignores the sets outright: nothing is quoted at all.
	if got := tracedWord(t, "a*b", TraceMetacharacters{Anywhere: "*"}); got != `'a*b'` {
		t.Fatalf("QuoteShell with the set = %s", got)
	}
	got := traceOf(t, "set -x; echo 'a*b'", permissive(), Diagnostics{
		TraceQuoting: QuoteNever, TraceMetacharacters: meta,
	})
	if !strings.Contains(got, "+ echo a*b\n") {
		t.Errorf("QuoteNever with a set = %q, want the word bare", got)
	}
}

// tracedTest runs one `[ … ]` command under an answer and gives the line back
// with the prefix trimmed. `[` is a builtin here, so the command runs.
func tracedTest(t *testing.T, src string, p TraceBareBracket) string {
	t.Helper()
	got := traceOf(t, "set -x; "+src, permissive(), Diagnostics{
		TraceQuoting:        QuoteShell,
		TraceMetacharacters: TraceMetacharacters{Anywhere: "*?[]{}"},
		TraceBareBracket:    p,
	})
	// The first line only: a command that fails to run says so on the same
	// stream, and the trace is what is under test.
	line, _, _ := strings.Cut(strings.TrimSpace(got), "\n")
	return strings.TrimSpace(strings.TrimPrefix(line, "+ "))
}

// TestTheBracketsOfATestAreTheOneExemption, and every row below rules out a
// simpler rule that fits some of the evidence — see docs/spec/prompt.md.
func TestTheBracketsOfATestAreTheOneExemption(t *testing.T) {
	for _, tc := range []struct {
		name, src             string
		quoted, cmdWord, pair string
	}{
		{
			"the whole test",
			`[ 1 -lt 2 ]`,
			`'[' 1 -lt 2 ']'`, `[ 1 -lt 2 ']'`, `[ 1 -lt 2 ]`,
		},
		{
			// The `[` is bare with no closer in sight, so what is matched is
			// the word and not the construct.
			"with no closing bracket at all",
			`[ 1 -lt 2 x`,
			`'[' 1 -lt 2 x`, `[ 1 -lt 2 x`, `[ 1 -lt 2 x`,
		},
		{
			// Only the final operand in the pair answer: an interior `]` is
			// an argument like any other.
			"an interior closing bracket is not the closer",
			`[ -n ']' ]`,
			`'[' -n ']' ']'`, `[ -n ']' ']'`, `[ -n ']' ]`,
		},
		{
			// A command word that is not `[` is quoted under every answer,
			// which is what says this is not "a command word is exempt".
			"a command word that is not a bracket",
			`'a[b' x`,
			`'a[b' x`, `'a[b' x`, `'a[b' x`,
		},
		{
			// And the character on its own is not exempt either.
			"a closing bracket as the command word",
			`']' z`,
			`']' z`, `']' z`, `']' z`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []struct {
				p    TraceBareBracket
				want string
			}{
				{TraceBracketQuotedLikeAnyWord, tc.quoted},
				{TraceBracketCommandWordBare, tc.cmdWord},
				{TraceBracketPairBare, tc.pair},
			} {
				if got := tracedTest(t, tc.src, a.p); got != a.want {
					t.Errorf("%s under %d = %q, want %q", tc.src, a.p, got, a.want)
				}
			}
		})
	}
}
