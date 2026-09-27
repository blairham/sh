// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The table this seam carries is **empty**, so nothing about the shell can
// prove the seam is there: an empty table and no table at all produce
// byte-identical behavior over every snippet anyone can write. That is what
// makes these tests the row rather than an extra on it — without them #4815 is
// unfalsifiable, and every later row of #4814 would be built on a seam nobody
// had shown existed.
//
// So the assertions are on the **seam itself**: a fixture axis is put in the
// table, and the parse has to follow it across an `emulate`, both ways round,
// and back again at the restore.
//
// `CStyleFor` is the fixture's field and **nothing here is a measurement of
// it**. It was chosen because it decides whether a snippet parses at all,
// which is what makes the assertion a parse rather than a field read; what any
// real mode does with it is a question for the row that measures it.

// fixtureGrammar installs a table for the duration of one test.
//
// The real table is put back by the cleanup rather than emptied, so a test
// that ran after a row of #4814 had landed restores that row's table and not
// a blank one.
func fixtureGrammar(t *testing.T, axes ...grammarAxis) {
	t.Helper()
	was := emulationGrammar
	emulationGrammar = axes
	t.Cleanup(func() { emulationGrammar = was })
}

// cStyleForFixture is the axis the tests below install: zsh's own mode keeps
// the construct and the three sh-family modes have it taken away.
func cStyleForFixture() grammarAxis {
	return grammarFlag("CStyleFor", func(d *syntax.Dialect) *bool { return &d.CStyleFor },
		map[string]bool{"zsh": true, "sh": false, "ksh": false, "csh": false})
}

// cStyleForSnippet parses with the runner's grammar as it stands and reports
// whether the construct is still in it.
func cStyleForSnippet(t *testing.T, r *interp.Runner) bool {
	t.Helper()
	if r.Dialect == nil {
		t.Fatal("the runner has no dialect, so there is no grammar to read")
	}
	_, err := syntax.Parse("for ((i = 0; i < 1; i++)); do :; done\n", *r.Dialect)
	return err == nil
}

// TestAnEmulationMovesTheGrammarAndPutsItBack is #4815's own bar: the seam
// exists, the parse follows it in both directions, and a scope that saved the
// option table puts the grammar back with it.
//
// Every assertion is a **parse**, not a field read. A test that read the field
// back would pass against a seam that wrote it somewhere the parser never
// looks, which is precisely the failure "the mode never reaches the grammar"
// already is.
func TestAnEmulationMovesTheGrammarAndPutsItBack(t *testing.T) {
	fixtureGrammar(t, cStyleForFixture())

	r := optionStateRunner(t)
	shared := r.Dialect
	if !cStyleForSnippet(t, r) {
		t.Fatal("the construct does not parse before any emulation, so the fixture cannot show a change")
	}

	applyEmulation(r, "sh", false)
	if cStyleForSnippet(t, r) {
		t.Error("the construct still parses under emulate sh, so the mode did not reach the grammar")
	}
	if r.Dialect == shared {
		t.Error("the dialect was written through rather than replaced: a subshell holds the same pointer, " +
			"and the front end has nothing but the pointer to notice a grammar by")
	}
	if !shared.CStyleFor {
		t.Error("the dialect the runner started with was written through, so a cloned runner's grammar moved too")
	}

	// The other direction, which is the half a narrowing table gets wrong:
	// zsh's own mode has to put the construct back rather than leave whatever
	// the last mode wrote.
	applyEmulation(r, "zsh", false)
	if !cStyleForSnippet(t, r) {
		t.Error("the construct does not parse again under emulate zsh, so the grammar only narrows")
	}

	// And the restore at the return, which is what a function's `emulate -L`
	// and an `emulate -c` both end with.
	saved := saveOptionState(r)
	applyEmulation(r, "sh", false)
	if cStyleForSnippet(t, r) {
		t.Fatal("the construct still parses under emulate sh on the second pass")
	}
	saved.restore(r)
	if !cStyleForSnippet(t, r) {
		t.Error("a restored option state left the emulated grammar standing, so a scope puts back the mode " +
			"and not what the mode reads with")
	}
}

// TestEveryGrammarAxisAnswersEveryMode is the completeness guard on the table.
//
// An axis silent about a mode leaves its field wherever the *previous*
// emulation left it, so `emulate sh; emulate zsh` would not be the shell it
// started as — and nothing about that reads as a bug from inside zsh's own
// mode, which is where almost every test looks.
//
// The committed table is empty, so the check over it passes by having nothing
// to look at. That is the same line of output as a check that passed, so the
// holed fixture below is not decoration: it is the positive this instrument
// has to be seen to produce before its silence over the real table means
// anything.
func TestEveryGrammarAxisAnswersEveryMode(t *testing.T) {
	if holes := grammarTableHoles(emulationGrammar); len(holes) > 0 {
		t.Errorf("the committed table has holes in it: %v", holes)
	}

	holed := grammarFlag("CStyleFor", func(d *syntax.Dialect) *bool { return &d.CStyleFor },
		map[string]bool{"zsh": true, "sh": false})
	holes := grammarTableHoles([]grammarAxis{holed})
	if len(holes) != 2 {
		t.Fatalf("a fixture answering two of the four modes reported %d holes (%v), want 2", len(holes), holes)
	}

	// And the other half of the same falsifier: an axis answering every mode
	// is quiet, so the check is reporting the hole rather than the axis.
	whole := cStyleForFixture()
	if holes := grammarTableHoles([]grammarAxis{whole}); len(holes) > 0 {
		t.Errorf("an axis answering every mode was reported as holed: %v", holes)
	}
}

// TestAModeTheTableDoesNotAnswerLeavesTheGrammarAlone pins what the hole above
// actually costs, so the guard's reason is in the tree rather than only in its
// comment.
func TestAModeTheTableDoesNotAnswerLeavesTheGrammarAlone(t *testing.T) {
	fixtureGrammar(t, grammarFlag("CStyleFor", func(d *syntax.Dialect) *bool { return &d.CStyleFor },
		map[string]bool{"sh": false}))

	r := optionStateRunner(t)
	applyEmulation(r, "sh", false)
	if cStyleForSnippet(t, r) {
		t.Fatal("the axis did not fire for the one mode it answers")
	}
	applyEmulation(r, "zsh", false)
	if cStyleForSnippet(t, r) {
		t.Error("a mode the axis does not answer put the construct back, so the carry-over this guards is not real")
	}
}
