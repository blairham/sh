// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The reading is three fields of syntax.Dialect and one question, so the
// thing to pin is that they cannot part: the getter reads one of them, and a
// setter that moved two would make it answer for a dialect that does not
// exist.
func TestTheCasePatternListReadingMovesAsOne(t *testing.T) {
	for _, on := range []bool{true, false} {
		d := syntax.Core()
		d.CasePatternListSpansBlanks = !on
		d.CasePatternListSpansNewlines = !on
		d.CasePatternMayBeEmpty = !on
		// testrunner:bare — nothing here runs a line; the subject is the
		// dialect pointer, and a directory of its own would only hide that.
		r := &interp.Runner{Dialect: &d}
		shared := r.Dialect

		r.SetCasePatternListReadAsOneWord(on)
		got := *r.Dialect
		if got.CasePatternListSpansBlanks != on ||
			got.CasePatternListSpansNewlines != on ||
			got.CasePatternMayBeEmpty != on {
			t.Errorf("setting the reading to %v left blanks=%v newlines=%v empty=%v",
				on, got.CasePatternListSpansBlanks, got.CasePatternListSpansNewlines,
				got.CasePatternMayBeEmpty)
		}
		if r.CasePatternListReadAsOneWord() != on {
			t.Errorf("the reading answers %v after being set to %v",
				r.CasePatternListReadAsOneWord(), on)
		}
		if r.Dialect == shared {
			t.Error("the dialect was written through rather than replaced: a subshell holds the same " +
				"pointer, and the front end has nothing but the pointer to notice a grammar by")
		}
	}
}

// And the other half: a dialect already holding the answer is left alone, so
// a runner whose grammar has not moved does not hand the front end a fresh
// pointer on every option word.
func TestSettingTheCasePatternListReadingItAlreadyHasSwapsNothing(t *testing.T) {
	d := syntax.Core()
	d.CasePatternListSpansBlanks = true
	d.CasePatternListSpansNewlines = true
	d.CasePatternMayBeEmpty = true
	// testrunner:bare — the subject is the dialect pointer and nothing runs.
	r := &interp.Runner{Dialect: &d}
	shared := r.Dialect

	r.SetCasePatternListReadAsOneWord(true)
	if r.Dialect != shared {
		t.Error("the dialect was replaced although nothing about it moved")
	}
}

// A partly-moved dialect is put right rather than read as agreeing, which is
// the case the early return has to get right: reading one field and writing
// three means the two have to agree about what "already holds it" is.
func TestAPartlyMovedCasePatternListReadingIsCompleted(t *testing.T) {
	d := syntax.Core()
	d.CasePatternListSpansBlanks = true
	d.CasePatternListSpansNewlines = false
	d.CasePatternMayBeEmpty = false
	// testrunner:bare — the subject is the dialect pointer and nothing runs.
	r := &interp.Runner{Dialect: &d}

	r.SetCasePatternListReadAsOneWord(true)
	if got := *r.Dialect; !got.CasePatternListSpansNewlines || !got.CasePatternMayBeEmpty {
		t.Errorf("a dialect holding one of the three was left holding one: newlines=%v empty=%v",
			got.CasePatternListSpansNewlines, got.CasePatternMayBeEmpty)
	}
}
