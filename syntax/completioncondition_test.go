// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
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

// The two completion-context tests are one-operand conditions where the flag
// is on, and the operand is read the way a pattern operand is rather than the
// way an option name is.
func TestTheCompletionConditionsAreOneOperandTests(t *testing.T) {
	for _, src := range []string{
		"[[ -prefix : ]]\n",
		"[[ -prefix 'ab' ]]\n",
		"[[ -prefix //(a|b)/ ]]\n",
		"[[ -suffix : ]]\n",
		"[[ -prefix x && -n y ]]\n",
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
	d := completionConds()
	for _, src := range []string{
		"[[ -prefix ]]\n",
		"[[ -suffix ]]\n",
		"[[ -prefix && -n x ]]\n",
		"[[ -prefix || -n x ]]\n",
		"[[ ( -prefix ) ]]\n",
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
