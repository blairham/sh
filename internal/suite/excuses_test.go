// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"reflect"
	"testing"
)

// TestOnlyOurOwnSentencesAreCounted is the rule that keeps this reportable.
// A line counts because it carries a phrase this project wrote, never because
// of its shape — a rule like "two colons in it" would sweep up whatever the
// file printed and call it our diagnostic.
func TestOnlyOurOwnSentencesAreCounted(t *testing.T) {
	mine := []string{
		"f.tests: line 3: hash: -l: not implemented yet",
		"f.tests: line 9: x: readonly variable",
		"one: two: three: a line of a file's own with colons in it",
		"shared",
	}
	theirs := []string{"shared"}
	got := RankExcuses(excuses(mine, theirs))
	want := []Excuse{
		{Phrase: "not implemented yet", Lines: 1},
		{Phrase: "readonly variable", Lines: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranked %v, want %v", got, want)
	}
}

// TestALineTheReferenceAlsoPrintedIsNotOurs. The count is of what we said and
// it did not; a diagnostic both shells produce is agreement, and counting it
// would rank the areas where this shell is right alongside the ones where it
// refuses.
func TestALineTheReferenceAlsoPrintedIsNotOurs(t *testing.T) {
	line := "f.tests: line 1: x: command not found"
	if got := RankExcuses(excuses([]string{line}, []string{line})); got != nil {
		t.Errorf("a line both shells printed was counted as ours: %v", got)
	}
	if got := RankExcuses(excuses([]string{line, line}, []string{line})); len(got) != 1 || got[0].Lines != 1 {
		t.Errorf("two of ours against one of theirs counted %v, want one line", got)
	}
}

// TestALineCountsOnce keeps the column a count of lines rather than of
// mentions, so it can be read against the differing-line figure beside it.
func TestALineCountsOnce(t *testing.T) {
	got := RankExcuses(excuses([]string{"f: usage: -x: invalid option"}, nil))
	if len(got) != 1 || got[0].Lines != 1 {
		t.Errorf("a line carrying two phrases counted %v", got)
	}
}
