// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A builtin operand's subscript that is whitespace and nothing else, which is
// what `printf -v "a[$k]"` is once a `$k` holding a tab or two spaces has gone
// in.
//
// For an *indexed* array the characters do not matter — a blank subscript is
// the empty expression whichever whitespace it was written with, which is
// Semantics.BlankArithSubscriptIsTheEmptyExpression and is measured next door.
// For a *keyed* one the text is the key, so collapsing it to a single space
// stores the element under a character the script never wrote — and leaves the
// one an ordinary `a[$k]=v` had already put under the real key standing beside
// it, which is the shape that makes this silent (#4175).

// blankOpGrammar is what the probes need: a subscript, an array literal, and a
// declaration utility to make the keyed table with.
func blankOpGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.DeclarationUtilities = map[string]bool{"typeset": true}
}

// blankOpSetup turns on the one option the store route needs and answers the
// array questions the rows are not about.
func blankOpSetup(r *Runner) {
	s := *r.Semantics
	arraySemantics(&s)
	s.PrintfAssignsWithV = Yes
	r.Semantics = &s
}

func TestABlankOperandSubscriptKeepsTheCharactersItWasWrittenWith(t *testing.T) {
	for _, c := range []struct {
		name string
		key  string
	}{
		{"two spaces", "  "},
		{"a tab", "\t"},
		{"a space and a tab", " \t"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// An ordinary `a[$k]=v` first, which does not go through an
			// operand at all, and the operand store second. **The count is
			// the assertion**: reading the element back through `$k` cannot
			// discriminate, because a collapse that reaches the store
			// reaches the read beside it and the two agree on the wrong key.
			// Two elements is the operand having written somewhere else.
			src := "typeset -A a; k='" + c.key + "'; " +
				`a[$k]=Y; printf -v "a[$k]" '%s' X; ` +
				`printf '[%s]' "${a[$k]}" "${#a[@]}"`
			out, st := runGrammar(t, src, blankOpGrammar, blankOpSetup)
			if out != "[X][1]" || st != 0 {
				t.Errorf("%s = %q (status %d), want \"[X][1]\" at 0", src, out, st)
			}
		})
	}
}

// The neighbor this must not disturb: a subscript that is *one* space is still
// one space, so the blank-versus-empty distinction the trimming protects is
// untouched.
func TestASingleSpaceOperandSubscriptIsStillOneSpace(t *testing.T) {
	const src = `typeset -A a; k=' '; a[$k]=Y; printf -v 'a[ ]' '%s' X; ` +
		`printf '[%s]' "${a[$k]}" "${#a[@]}"`
	out, st := runGrammar(t, src, blankOpGrammar, blankOpSetup)
	if out != "[X][1]" || st != 0 {
		t.Errorf("%s = %q (status %d), want \"[X][1]\" at 0", src, out, st)
	}
}
