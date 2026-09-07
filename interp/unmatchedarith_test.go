// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// UnmatchedArithSubst words `$((` and `$[` the input ran out inside, and its
// empty answer is the substrate's own sentence rather than the quote's.
//
// That fallback is the part with a reason. An opener with no wording of its
// own falls through to UnmatchedQuote at this site, which for this construct
// would make a shell say something about a quote where the script wrote no
// quote at all. Every preset in the panel states an answer, so the fallback
// is what the core and a new dialect get.
//
// Named for the field rather than for a shell, which is this package's rule.
// What each shell actually prints is pinned in its own package.
func TestUnmatchedArithSubstWordsBothSpellingsAndFallsBackToTheSubstrate(t *testing.T) {
	bracket := syntax.Core()
	bracket.DollarBracketArith = true
	for _, tc := range []struct {
		why, src string
		d        syntax.Dialect
		diag     Diagnostics
		want     string
	}{
		{
			why:  "the closer comes from the construct, so one wording serves both spellings",
			src:  "echo $((1+2",
			d:    syntax.Core(),
			diag: Diagnostics{UnmatchedArithSubst: "no matching `%[2]s'"},
			want: "no matching `)'",
		},
		{
			why:  "and the bracketed spelling gets its own bracket from the same wording",
			src:  "echo $[1+2",
			d:    bracket,
			diag: Diagnostics{UnmatchedArithSubst: "no matching `%[2]s'"},
			want: "no matching `]'",
		},
		{
			why:  "the word is available too, for a dialect that quotes it",
			src:  "v=$((1+2",
			d:    syntax.Core(),
			diag: Diagnostics{UnmatchedArithSubst: "near `%[3]s'"},
			want: "near `v=$((1+2'",
		},
		{
			why: "empty is the substrate's sentence, not the quote's",
			src: "echo $((1+2",
			d:   syntax.Core(),
			// A quote wording is set and deliberately not used: if this
			// construct fell through to it the answer would be that text.
			diag: Diagnostics{UnmatchedQuote: "unmatched %[1]s"},
			want: "unterminated arithmetic substitution",
		},
		{
			why:  "and the same for the bracketed spelling",
			src:  "echo $[1+2",
			d:    bracket,
			diag: Diagnostics{UnmatchedQuote: "unmatched %[1]s"},
			want: "unterminated arithmetic substitution",
		},
		{
			why:  "a quote really does still get the quote wording",
			src:  `echo "abc`,
			d:    syntax.Core(),
			diag: Diagnostics{UnmatchedQuote: "unmatched %[1]s"},
			want: `unmatched "`,
		},
	} {
		_, err := syntax.Parse(tc.src, tc.d)
		if err == nil {
			t.Errorf("%s: %q parsed, want a refusal", tc.why, tc.src)
			continue
		}
		if got := tc.diag.ParseFailure(err); got != tc.want {
			t.Errorf("%s: %q:\n got %q\nwant %q", tc.why, tc.src, got, tc.want)
		}
	}
}

// The line convention this construct takes, which is the whole reason #1086
// was a second measurement rather than a rider on #1023.
//
// A dialect that reports an unmatched `$(` at the line after the input's last
// reports `$((` at the **opener's** line. Nothing new decides that: the two
// flags ParseFailureLine already reads are keyed on the opener, and `$((` is
// not one of the three that hold a program, so it takes the same branch a
// quote takes — which is what that dialect prints. Adding it to the other set
// is the mistake that would move the line, and this is the test that catches
// it.
func TestAnUnmatchedArithSubstTakesTheOpenerLineNotTheEndOfInput(t *testing.T) {
	const src = "echo $((1+2\n"
	_, err := syntax.Parse(src, syntax.Core())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	// The two conventions give different numbers here — the opener is on
	// line 1 and the input ran out on line 2 — so neither can pass for the
	// other.
	atOpener := Diagnostics{UnmatchedArithSubst: "x", UnmatchedReportedAtOpener: true}
	if got := atOpener.ParseFailureLine(err); got != 1 {
		t.Errorf("reported at the opener: line %d, want 1", got)
	}
	atEOF := Diagnostics{UnmatchedArithSubst: "x"}
	if got := atEOF.ParseFailureLine(err); got != 2 {
		t.Errorf("reported where the input ran out: line %d, want 2", got)
	}
	// And the flag for the constructs that hold a program must not reach
	// this one: a dialect with both set answers the opener, because `$((`
	// is not `$(`.
	both := Diagnostics{
		UnmatchedArithSubst:       "x",
		UnmatchedReportedAtOpener: true,
		CmdSubstUnmatchedAtEnd:    true,
	}
	if got := both.ParseFailureLine(err); got != 1 {
		t.Errorf("with CmdSubstUnmatchedAtEnd also set: line %d, want 1 — "+
			"`$((` has been folded into the `$(` set", got)
	}
	// The control: the same dialect answers the end of input for `$(`.
	const subst = "echo $(echo hi\n"
	_, serr := syntax.Parse(subst, syntax.Core())
	if serr == nil {
		t.Fatalf("%q parsed, want a refusal", subst)
	}
	if got := both.ParseFailureLine(serr); got != 2 {
		t.Errorf("`$(` under the same dialect: line %d, want 2", got)
	}
}
