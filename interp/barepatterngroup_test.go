// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Both halves of the pair are written and both are readable back, which is
// the property an option namespace that spells them as two names needs: one
// name turned on inside a state the other name set must still be able to say
// what it is.
func TestTheBarePatternGroupPairIsWrittenAndReadBack(t *testing.T) {
	for _, tc := range []struct{ anywhere, insideAWord bool }{
		{true, true}, {true, false}, {false, true}, {false, false},
	} {
		d := syntax.Core()
		d.PatternAlternation = !tc.anywhere
		d.BarePatternGroupInsideAWord = !tc.insideAWord
		// testrunner:bare — nothing here runs a line; the subject is the
		// dialect pointer, and a directory of its own would only hide that.
		r := &interp.Runner{Dialect: &d}
		shared := r.Dialect

		r.SetBarePatternGroups(tc.anywhere, tc.insideAWord)
		if got := r.BarePatternGroupsOpenAnywhere(); got != tc.anywhere {
			t.Errorf("anywhere reads %v after being set to %v", got, tc.anywhere)
		}
		if got := r.BarePatternGroupsOpenInsideAWord(); got != tc.insideAWord {
			t.Errorf("inside-a-word reads %v after being set to %v", got, tc.insideAWord)
		}
		if r.Dialect == shared {
			t.Error("the dialect was written through rather than replaced: a subshell holds the same " +
				"pointer, and the front end has nothing but the pointer to notice a grammar by")
		}
	}
}

// And a pair already in place is left alone, so a runner whose grammar has
// not moved does not hand the front end a fresh pointer on every option word.
func TestSettingTheBarePatternGroupPairItAlreadyHasSwapsNothing(t *testing.T) {
	d := syntax.Core()
	d.PatternAlternation = true
	d.BarePatternGroupInsideAWord = false
	// testrunner:bare — the subject is the dialect pointer and nothing runs.
	r := &interp.Runner{Dialect: &d}
	shared := r.Dialect

	r.SetBarePatternGroups(true, false)
	if r.Dialect != shared {
		t.Error("the dialect was replaced although nothing about it moved")
	}

	// The other half of the same guard: moving only the second field is a
	// move. A setter comparing one of the two would skip this.
	r.SetBarePatternGroups(true, true)
	if r.Dialect == shared {
		t.Error("only the second field moved and the dialect was not replaced, so the front end " +
			"would read the rest of the program the old way")
	}
}
