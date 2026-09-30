// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The four axes where this shell's expander reads a character differently from
// the other two, asserted as preset values.
//
// The values rather than the behavior, for the reason
// TestZshAnswersNoToHistoryExpansionInAScript gives: zsh has no `set -o
// history`, so a zsh script can never turn the list on and no transcript this
// package can run reaches the engine at all. The behavior each value produces
// is pinned in internal/histexpand, where the engine takes the vector
// directly — TestAQuoteAfterTheEventCharacter,
// TestAnEventCharacterInsideAnEventName, TestABracedEventReference and
// TestTheLastWordEndsTheDesignator.
//
// Measured 2026-09-18 on zsh 5.9.2 through a pseudo-terminal with a two-row
// prompt, against a one-line list:
//
//	echo "T!'x E"        T!'x E                  the `!` is text
//	printf %s T!`echo z`E  T!zE                  and before a backquote too
//	X!ab!cdY             event not found: ab!    the name stops after the `!`
//	!{x}                 event not found: x      the braced form
//	!!:$-3               no such word in event   a `$` does not end it
//	!!:$-                no such word in event
//
// bash 5.3.20 in a script and ksh93u+ at a prompt answer the opposite of every
// one, which is what makes each of them an axis rather than the engine's.
func TestZshReadsAnEventNameItsOwnWay(t *testing.T) {
	s := zsh.Semantics()
	for _, c := range []struct {
		name string
		got  interp.Answer
		want interp.Answer
	}{
		{"HistoryQuoteEndsAnEventReference", s.HistoryQuoteEndsAnEventReference, interp.Yes},
		{"HistoryEventCharClosesAnEventName", s.HistoryEventCharClosesAnEventName, interp.Yes},
		{"HistoryBracedEventReference", s.HistoryBracedEventReference, interp.Yes},
		{"HistoryLastWordEndsTheDesignator", s.HistoryLastWordEndsTheDesignator, interp.No},
	} {
		if c.got != c.want {
			t.Errorf("%s is %v, want %v", c.name, c.got, c.want)
		}
	}
	// And the two this shell shares with one of the others, so that a preset
	// moved for a different column does not quietly take zsh with it.
	if got := s.HistoryFirstWordEndsARange; got != interp.Yes {
		t.Errorf("HistoryFirstWordEndsARange is %v, want Yes — `!!:1-^` is `a` here as it is in bash", got)
	}
	if got := s.HistoryWordwiseSubstitutionModifier; got != interp.No {
		t.Errorf("HistoryWordwiseSubstitutionModifier is %v, want No — `:G` is `illegal modifier: G` here", got)
	}
}

// A backslash in a history substitution's replacement escapes what follows
// it, and it happens **twice**.
//
// See Semantics.HistorySubstitutionUnescapesTheReplacement for the panel.
// Measured 2026-09-30 at a prompt: `!!:s/o/\\\\X/` written with four
// backslashes comes back with one, where bash 5.3.20 and 3.2.57 come back
// with four.
//
// **The expansion is run and not only the axis read**, which is what a
// surviving mutant asked for: the engine's own tests build their
// histexpand.Chars by hand, so an axis that was never wired into
// Runner.HistoryChars passed every one of them. This row goes through the
// runner, which is the only place the answer and the expander meet.
func TestZshSubstitutionUnescapesTheReplacementTwice(t *testing.T) {
	if got := zsh.Semantics().HistorySubstitutionUnescapesTheReplacement; got != interp.Yes {
		t.Errorf("HistorySubstitutionUnescapesTheReplacement is %v, want Yes", got)
	}
	sem, diag, d := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &diag, Name: "zsh",
		// Set because the guard in internal/dialecttest asks for it: a nil
		// Dialect is the core, so any nested parse would run as a shell this
		// row is not about.
		Dialect: &d,
	}
	zsh.Apply(r)
	// Always, not the route-gated entry point: zsh does not expand history
	// in a script, so the gated one returns the line untouched and the row
	// would pass whatever the rule did.
	res, err := r.ExpandHistoryAlways(`echo !!:s/o/\\\\X/`, []string{"echo one two one"}, 1)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if want := `echo ech\X one two one`; res.Line != want {
		t.Errorf("expanded to %q, want %q", res.Line, want)
	}
}
