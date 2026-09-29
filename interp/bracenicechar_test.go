// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runCharRange expands one word with the axes a **character** range needs,
// including the wide reading that is the only one able to produce an
// unprintable character.
func runCharRange(t *testing.T, src string, wide Answer) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.BraceExpansion = Yes
	sem.BraceCharRangeSpansAnyCharacter = wide
	sem.BraceRangePadsToEndpointWidth = Yes
	sem.BraceRangeStepPadsTheRange = Yes
	sem.BraceRangeStepSignHonored = No
	sem.BraceRangeNegativeStepReverses = Yes
	sem.BraceRangeZeroStepCountsAsOne = Yes
	sem.BraceRangeMissingEndCountsFromZero = No
	sem.BraceRangeNumberMayCarryAPlus = Yes
	sem.BraceRangeThatCannotBeCounted = BraceRangeFailureKeepsTheWord
	sem.BraceBodyIsACharacterClass = No
	// The produced text holds `^` and `\`, so a row would otherwise reach
	// the question of whether brace output re-enters the word as shell text.
	// Answered here because it is not this test's subject.
	sem.BraceOutputRereadAsText = No
	sem.LoneDashIsAnOption = No
	// Three more the *rows* reach and the subject does not: the endpoints
	// are written as `$'...'` exactly as the suite file writes them, so the
	// range's endpoint-expansion question and two of `$'...'`'s own are on
	// the way in. Answered so that a refusal from any of them cannot be
	// mistaken for this question's.
	sem.BraceRangeEndpointsExpanded = Yes
	sem.DollarSingleUnicodeEscapes = Yes
	sem.DollarSingleUnknownEscape = DollarSingleUnknownKeepsBackslash
	d := syntax.Core()
	d.DollarSingleQuote = true
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dialect: &d,
		Stdout: &buf, Stderr: &buf,
	})
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// A character range writes an unprintable character in the measured shell's
// escape form rather than as itself.
//
// Measured 2026-09-29 on zsh 5.9.2, each row a range of **one** element so
// that the rendering is separated from everything a longer range does — and
// so that it is separated from `print`, which writes the same character
// raw. See Runner.niceRangeChar and instruments.md §4.
func TestACharacterRangeWritesAnUnprintableCharacterAsAnEscape(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The two with C escapes.
		{"tab", `echo {$'\x09'..$'\x09'}`, `\t`},
		{"newline", `echo {$'\x0a'..$'\x0a'}`, `\n`},
		// The rest of C0 is `^` plus the byte or'd with 0x40. A NUL is not
		// here: `$'\x00'` reaches a question of its own, and the row is in
		// dialect/zsh with the C1 ones.
		{"one", `echo {$'\x01'..$'\x01'}`, "^A"},
		{"bell", `echo {$'\x07'..$'\x07'}`, "^G"},
		{"vertical tab", `echo {$'\x0b'..$'\x0b'}`, "^K"},
		{"carriage return", `echo {$'\x0d'..$'\x0d'}`, "^M"},
		{"unit separator", `echo {$'\x1f'..$'\x1f'}`, "^_"},
		{"delete", `echo {$'\x7f'..$'\x7f'}`, "^?"},
		// The C1 range and everything above Latin-1 need escapes and a
		// character reading this harness does not answer; those rows are in
		// dialect/zsh, where the whole vector is.
		// Printable characters are themselves, which is the bound: a space
		// and a backslash are both printable and neither is escaped.
		{"space", `echo {$'\x20'..$'\x20'}`, " "},
		{"bang", `echo {$'\x21'..$'\x21'}`, "!"},
		{"backslash", `echo {$'\x5c'..$'\x5c'}`, `\`},
		{"tilde", `echo {$'\x7e'..$'\x7e'}`, "~"},
		// The letters reading's widest gap, every character of which is
		// printable — this function is identity on everything that reading
		// can produce.
		{"the case gap", `echo {X..c}`, "X Y Z [ " + `\` + " ] ^ _ ` a b c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runCharRange(t, tc.src, Yes); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The narrow reading cannot produce an unprintable character at all, so the
// rendering is unreachable from it and every row above that it *can* reach
// answers the same either way.
//
// This is the row that says the escape form is not a second rule bolted on
// to every brace: with the wide reading off, a range between two characters
// that are not both letters is not a range.
func TestTheNarrowCharacterRangeReadingNeverReachesTheEscapes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo {$'\x01'..$'\x01'}`, `{..}`},
		// Letters still count, and they are printable.
		{`echo {X..c}`, "X Y Z [ " + `\` + " ] ^ _ ` a b c"},
		{`echo {a..c}`, "a b c"},
	} {
		got := runCharRange(t, tc.src, No)
		if tc.want == `{..}` {
			// The word stands; what is between the braces is the raw
			// character, which is not worth spelling in a table.
			if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
				t.Errorf("%s = %q, want the word left alone", tc.src, got)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Only a character range renders. An alternative, a class and a bare word
// all write the character raw, measured on the same shell — so a rendering
// put in the shared span builder would have changed three constructs that
// agree today.
func TestOnlyACharacterRangeRendersTheEscape(t *testing.T) {
	raw := "\x01"
	for _, tc := range []struct{ name, src, want string }{
		{"an alternative", `echo {$'\x01',b}`, raw + " b"},
		{"a bare word", `echo $'\x01'`, raw},
		{"a numeric range", `echo {1..2}`, "1 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runCharRange(t, tc.src, Yes); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
