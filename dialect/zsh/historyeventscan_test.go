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
