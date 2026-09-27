// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// braceProduced runs one snippet under a vector answering
// BraceScanReadsProducedText the given way, on the road that finds the braces
// in the fields the word came to, with every other brace axis the rows reach
// pinned so that nothing here is decided by a refusal.
func braceProduced(t *testing.T, src string, reads Answer) (string, int) {
	t.Helper()
	out, st := run(t, emptyAltCounter+src+"\n", func(r *Runner) {
		s := *r.Semantics
		s.BraceExpansion = Yes
		s.BraceFanExpandsEachNameOnItsOwn = No
		s.BraceScanReadsProducedText = reads
		s.BraceBodyReadAfterExpansion = reads
		s.BraceOutputRereadAsText = No
		s.BraceRescanEntersFailedGroup = No
		s.BraceEmptyAlternativeIsAField = Yes
		s.BraceStopsFieldSplitting = Yes
		s.BraceRangeEndpointsExpanded = Yes
		r.Semantics = &s
	})
	return strings.TrimSpace(out), st
}

// Whether the braces an *expansion* produced are syntax at all.
//
// This is the question one level out from a produced comma inside a group the
// script wrote: there the delimiters are written and only the body is in
// doubt, here there is no written group to be the body of.
func TestBracesAnExpansionProduced(t *testing.T) {
	for _, tc := range []struct{ name, src, reads, text string }{
		{"a whole group", `e='{a,b}'; f $e`, `2 | [a] [b]`, `1 | [{a,b}]`},
		{"beside written text", `e='{a,b}'; f x$e`, `2 | [xa] [xb]`, `1 | [x{a,b}]`},
		{
			"and a written group behind it", `e='{a,b}'; f $e{c,d}`,
			`4 | [ac] [ad] [bc] [bd]`, `2 | [{a,b}c] [{a,b}d]`,
		},
		{
			"twice over", `e='{a,b}'; f $e$e`,
			`4 | [aa] [ab] [ba] [bb]`, `1 | [{a,b}{a,b}]`,
		},
		{
			"halves from two expansions", `e='{a,'; g='b}'; f $e$g`,
			`2 | [a] [b]`, `1 | [{a,b}]`,
		},
		{"a produced range", `e='{1..3}'; f $e`, `3 | [1] [2] [3]`, `1 | [{1..3}]`},
		{"a produced empty alternative", `e='{a,}'; f $e`, `2 | [a] []`, `1 | [{a,}]`},

		// The pairing rows. A brace closes only one of its own provenance,
		// which is what these three hold apart — and each fails a different
		// way under a rule that let them pair across.
		{
			"a produced closing brace closes nothing", `e='}'; f {a,b$e`,
			`1 | [{a,b}]`, `1 | [{a,b}]`,
		},
		{
			"a produced opening brace opens", `e='{'; f $e{a,b}`,
			`1 | [{{a,b}]`, `2 | [{a] [{b]`,
		},
		{
			"two produced braces pair", `e='{}'; f $e{a,b}`,
			`2 | [{}a] [{}b]`, `2 | [{}a] [{}b]`,
		},
		{
			"a produced opening brace in a written body", `e='{'; f {c,d$e}{a,b}`,
			`1 | [{c,d{}{a,b}]`, `4 | [ca] [cb] [d{a] [d{b]`,
		},

		// Quoting is what the two questions part over, and both rows are the
		// same either way: the *result* of a quoted expansion is not a brace
		// anywhere, and text the script wrote inside quotes is not one
		// either.
		{"a quoted expansion's result", `e='{a,b}'; f "$e"`, `1 | [{a,b}]`, `1 | [{a,b}]`},
		{"a quoted group", `f "{a,b}"`, `1 | [{a,b}]`, `1 | [{a,b}]`},
		{"a quoted list's element", `set -- '{a,b}'; f "$@"`, `1 | [{a,b}]`, `1 | [{a,b}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := braceProduced(t, tc.src, Yes); out != tc.reads || st != 0 {
				t.Errorf("reads: %s = %q status %d, want %q", tc.src, out, st, tc.reads)
			}
			if out, st := braceProduced(t, tc.src, No); out != tc.text || st != 0 {
				t.Errorf("text: %s = %q status %d, want %q", tc.src, out, st, tc.text)
			}
		})
	}
}

// And a group the script *did* write still reads the commas a quoted
// expansion produced, which is the one place the two questions give different
// answers about the same quoting.
func TestAQuotedExpansionInsideAWrittenGroup(t *testing.T) {
	const src = `e=a,b; f {"$e"}`
	if out, st := braceProduced(t, src, Yes); out != `2 | [a] [b]` || st != 0 {
		t.Errorf("reads: %s = %q status %d, want %q", src, out, st, `2 | [a] [b]`)
	}
	if out, st := braceProduced(t, src, No); out != `1 | [{a,b}]` || st != 0 {
		t.Errorf("text: %s = %q status %d, want %q", src, out, st, `1 | [{a,b}]`)
	}
}
