// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// completionOnTheThirdWord is the pty session's own line: `git sub a=b=c tail`
// with the cursor at the end of the third word, so there is a word **after**
// the cursor. completionFor cannot ask that — it puts the cursor at the end of
// the line — and without a word behind the cursor every `-between` row would
// be answered by the end pattern matching nothing, which is one rule standing
// in for two.
func completionOnTheThirdWord(t *testing.T, src string) []string {
	t.Helper()
	const line = "git sub a=b=c tail"
	r := bindkeyRunner(t, src)
	var out []string
	for _, c := range zsh.RunCompletion(r, t.Context(), "probewid", repl.Completion{
		Line:  line,
		Point: len("git sub a=b=c"),
		Start: len("git sub "),
		Word:  "a=b=c",
		Dir:   r.Dir,
	}) {
		out = append(out, c.Word)
	}
	return out
}

// The four completion-context conditions answered from inside a completion,
// which is the only place they answer at all.
//
// Measured on zsh 5.9.2, 2026-09-19, through a pseudo-terminal driven by the
// real shell's own `zpty`: a `zle -C` widget with `git sub a=b=c tail` typed
// and the cursor at the end of the third word, so `PREFIX` is `a=b=c`,
// `SUFFIX` is empty, `words` is `(git sub a=b=c tail)` and `CURRENT` is 3.
// The rows are that session's, one condition per row.
func TestTheCompletionConditionsAnswerInsideACompletion(t *testing.T) {
	for _, c := range []struct {
		name, cond string
		want       int
	}{
		// `-prefix` is `compset -P`'s test: anchored at the start of
		// `PREFIX`, and the operand is a pattern.
		{"-prefix a literal that begins the word", "-prefix a", 0},
		{"-prefix a pattern", "-prefix *=", 0},
		// The optional count picks which match `compset -P` would move and
		// cannot change whether there is one, so it cannot change the test.
		{"-prefix with a count in front", "-prefix 1 *=", 0},
		{"-prefix a literal that does not", "-prefix zz", 1},
		// Quoting decides whether the operand is a pattern or a literal,
		// exactly as it does for `==`. This is the control for the row above
		// it: same characters, different answer.
		{"-prefix a quoted pattern is a literal", "-prefix '*='", 1},
		// `-suffix` is `compset -S`'s test, and `SUFFIX` is empty here — see
		// compsys.go, where that is this editor's doing and not this file's.
		// An empty pattern matches it and nothing else does.
		{"-suffix against an empty suffix", "-suffix c", 1},
		{"-suffix an empty pattern", "-suffix ''", 0},
		// `-after` is `compset -N`'s test with only the start pattern: does
		// a word *before* the cursor match.
		{"-after the first word", "-after git", 0},
		{"-after the word before the cursor", "-after sub", 0},
		{"-after a word behind the cursor", "-after tail", 1},
		{"-after a pattern", "-after *u*", 0},
		{"-after a word that is not on the line", "-after zz", 1},
		// `-between` is the same test with an end pattern, and the three
		// rows below are what makes it a different operator rather than
		// `-after` with a word nobody reads: they differ only in the second
		// operand.
		{"-between with the end word after the cursor", "-between git tail", 0},
		{"-between with the end word before it", "-between git sub", 1},
		{"-between with an end word that is not there", "-between git zz", 0},
		// The end word has to be *after* the cursor and not at it, which is
		// the row that separates `>` from `>=`: the third word is the one
		// being completed.
		{"-between whose end word is the one being completed", "-between git a=b=c", 1},
		{"-between whose start word is behind the cursor", "-between tail zz", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionOnTheThirdWord(t, widgetOf(
				"[[ "+c.cond+" ]]; compadd -U -Q -- \"st=$?\"",
			))
			want := "st=" + map[int]string{0: "0", 1: "1"}[c.want]
			if len(got) != 1 || got[0] != want {
				t.Errorf("[[ %s ]] offered %q, want [%q]", c.cond, got, want)
			}
		})
	}
}

// And the condition does **not** move the word, which is the whole difference
// between it and the `compset` call it is the test of: the manual says the
// special parameters are not modified, and the pty session above read
// `PREFIX=a=b=c` and `IPREFIX=` back out after all fifteen conditions had
// run.
func TestACompletionConditionLeavesTheParametersAlone(t *testing.T) {
	got := completionOnTheThirdWord(t, widgetOf(
		"[[ -prefix *= ]]; [[ -after git ]]; [[ -between git tail ]]; "+
			"compadd -U -Q -- \"PREFIX=$PREFIX IPREFIX=$IPREFIX w=${(j:|:)words} C=$CURRENT\"",
	))
	want := "PREFIX=a=b=c IPREFIX= w=git|sub|a=b=c|tail C=3"
	if len(got) != 1 || got[0] != want {
		t.Errorf("offered %q, want [%q]", got, want)
	}
	// The control: the same pattern through `compset` *does* move it, so the
	// row above is a statement about the condition and not about the pattern
	// failing to match.
	moved := completionOnTheThirdWord(t, widgetOf(
		"compset -P '*='; compadd -U -Q -- \"PREFIX=$PREFIX IPREFIX=$IPREFIX\"",
	))
	if len(moved) != 1 || !strings.HasSuffix(moved[0], "PREFIX=c IPREFIX=a=b=") {
		t.Errorf("`compset -P` offered %q, want it to end PREFIX=c IPREFIX=a=b=", moved)
	}
}

// `zmodload zsh/complete` succeeds now that the module's last four features
// are here, and that is the user-visible half of #3042.
//
// Measured on zsh 5.9.2: `zmodload zsh/complete` is 0 and the module is in
// the listing afterwards. It was refused here — `after, between, prefix and
// suffix are not implemented yet` — for as long as the conditions were
// missing, because a condition has no word that runs it and so no way to tell
// a script about itself.
func TestTheCompleteModuleLoads(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/complete; echo st=$?; zmodload`)
	if st != 0 {
		t.Errorf("status = %d, want 0: %q", st, out)
	}
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "zsh/complete") {
		t.Errorf("said %q, want a load at 0 and the module in the listing", out)
	}
	// And the narrowed form asks the same question of one feature.
	out, st = answersRun(t, `zmodload -F zsh/complete c:between; echo st=$?`)
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("`-F c:between` said %q at %d, want st=0", out, st)
	}
}
