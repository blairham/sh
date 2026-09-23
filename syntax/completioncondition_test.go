// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

func completionConds() syntax.Dialect {
	d := syntax.Core()
	d.CompletionConditions = true
	// The operand row below is a *pattern* with a group in it, which is the
	// shape the two real occurrences have. Reading it needs the flag that
	// reads a group as part of a pattern, and the only preset with the
	// completion conditions has that one too.
	d.PatternAlternation = true
	return d
}

// The four completion-context tests parse where the flag is on, with the
// operand counts their arities allow, and the operand is read the way a
// pattern operand is rather than the way an option name is.
func TestTheCompletionConditionsAreOneOperandTests(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"[[ -prefix : ]]\n",
		"[[ -prefix 'ab' ]]\n",
		"[[ -prefix //(a|b)/ ]]\n",
		"[[ -suffix : ]]\n",
		"[[ -prefix x && -n y ]]\n",
		// The optional count in front of the pattern, and the two operators
		// the flag gained with it.
		"[[ -prefix 1 '*=' ]]\n",
		"[[ -suffix 2 '*=' ]]\n",
		"[[ -after x ]]\n",
		"[[ -between x y ]]\n",
		"[[ -between x y && -n z ]]\n",
	} {
		if _, err := syntax.Parse(src, completionConds()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
		if _, err := syntax.Parse(src, syntax.Core()); err == nil {
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// With no operand after it the word is an ordinary one and the condition is
// the bare-word test for non-emptiness — which every other unary operator in
// the table refuses instead. That difference is measured, and without it the
// flag would make one dialect refuse a line three other columns run.
func TestACompletionConditionWithNoOperandIsAWord(t *testing.T) {
	t.Parallel()
	d := completionConds()
	for _, src := range []string{
		"[[ -prefix ]]\n",
		"[[ -suffix ]]\n",
		"[[ -after ]]\n",
		"[[ -between ]]\n",
		"[[ -prefix && -n x ]]\n",
		"[[ -prefix || -n x ]]\n",
		"[[ -between && -n x ]]\n",
		"[[ ( -prefix ) ]]\n",
		"[[ ( -between ) ]]\n",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		// The core reads the same line the same way, the word being
		// ordinary there too.
		if _, err := syntax.Parse(src, syntax.Core()); err != nil {
			t.Errorf("%q: refused under the core: %v", src, err)
		}
		_ = f
	}
	// And the operators that do demand an operand still do, with or without
	// the flag: this is the pair's own rule and not a loosening of the
	// table.
	for _, src := range []string{"[[ -n ]]\n", "[[ -z ]]\n", "[[ -f ]]\n"} {
		if _, err := syntax.Parse(src, d); err == nil {
			t.Errorf("%q parsed; that operator demands its operand", src)
		}
	}
}

// TestACompletionConditionWithTheWrongArityParses is the half the parser hands
// on rather than answers. Measured on zsh 5.9.2, 2026-09-19, with `-n` for the
// parse and a run for the verdict — every one of these reads, and every one of
// them is `unknown condition: <op>` at status 2 when it runs, which is the
// node [Dialect.ConditionIsResolvedWhenItRuns] already had.
//
// The control rows are the counts that are *not* refused, so a reading that
// simply accepted everything would fail the test above rather than pass this
// one.
func TestACompletionConditionWithTheWrongArityParses(t *testing.T) {
	t.Parallel()
	d := completionConds()
	d.ConditionIsResolvedWhenItRuns = true
	for _, src := range []string{
		"[[ -prefix a b c ]]\n",
		"[[ -after a b ]]\n",
		"[[ -between a ]]\n",
		"[[ -between a b c ]]\n",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got := syntax.Print(f); got != strings.TrimSuffix(src, "\n") {
			t.Errorf("%q printed back as %q", src, got)
		}
	}
	// And the same counts are a parse failure where that flag is off, which
	// is the rule everywhere else in this table.
	off := completionConds()
	for _, src := range []string{"[[ -after a b ]]\n", "[[ -between a ]]\n"} {
		if _, err := syntax.Parse(src, off); err == nil {
			t.Errorf("%q parsed with the arity flag off", src)
		}
	}
}

// And a condition that parsed is printed back as it was read, operator first.
func TestACompletionConditionPrintsBackAsItWasRead(t *testing.T) {
	t.Parallel()
	d := completionConds()
	for _, src := range []string{
		"[[ -prefix //(a|b)/ ]]\n",
		"[[ -prefix 1 '*=' ]]\n",
		"[[ -after x ]]\n",
		"[[ -between x y ]]\n",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got := syntax.Print(f); got != strings.TrimSuffix(src, "\n") {
			t.Errorf("%q printed back as %q", src, got)
		}
	}
}
