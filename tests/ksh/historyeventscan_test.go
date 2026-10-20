// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// The one axis where this shell's expander reads a range's end differently
// from the other two, asserted as a preset value for the reason
// TestKshAnswersNoToHistoryExpansionInAScript gives: no ksh script can turn
// the list on, so no transcript this package can run reaches the engine.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01 through a pseudo-terminal with a
// two-row prompt, against `qqq a b c d e`:
//
//	!qqq:1-^     a b c d^                the `^` stays as text
//	!qqq:$-3     e-3                     and a `$` ends the designator
//	!qqq:$-      e-
//	!qqq:$*      e*
//
// bash 5.3.20 answers `a` to the first and agrees on the rest; zsh 5.9.2
// answers `a` to the first and refuses the rest. So the two questions split
// the panel two different ways, which is why they are two axes.
func TestKshReadsARangeEndItsOwnWay(t *testing.T) {
	s := ksh.Semantics()
	if got := s.HistoryFirstWordEndsARange; got != interp.No {
		t.Errorf("HistoryFirstWordEndsARange is %v, want No", got)
	}
	if got := s.HistoryLastWordEndsTheDesignator; got != interp.Yes {
		t.Errorf("HistoryLastWordEndsTheDesignator is %v, want Yes", got)
	}
	// And the three the panel's other split puts this column with bash on,
	// so that a preset moved for zsh does not take ksh93 with it.
	for _, c := range []struct {
		name string
		got  interp.Answer
	}{
		{"HistoryQuoteEndsAnEventReference", s.HistoryQuoteEndsAnEventReference},
		{"HistoryEventCharClosesAnEventName", s.HistoryEventCharClosesAnEventName},
		{"HistoryBracedEventReference", s.HistoryBracedEventReference},
		{"HistoryWordwiseSubstitutionModifier", s.HistoryWordwiseSubstitutionModifier},
	} {
		if c.got != interp.No {
			t.Errorf("%s is %v, want No", c.name, c.got)
		}
	}
}
