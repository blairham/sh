// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A `(` standing first in an expansion's pattern operand, which one dialect
// refuses while reading (#1430).
//
// The flag is named here and never a shell. Both sides are asserted, because
// asserting only the refusal asserts a default.
func TestAGroupOpeningAPatternOperandCanBeRefused(t *testing.T) {
	for _, src := range []string{
		`echo "${v#(a)}"`,
		`echo "${v%(b)}"`,
		`echo "${v##(a)}"`,
		`echo "${v%%(b)}"`,
		`echo "${v/(a)/Z}"`,
		`echo "${v/(a)}"`,
		// However many parentheses: the first one is what is refused.
		`echo "${v#((a))}"`,
	} {
		d := syntax.Core()
		d.GroupOpeningAPatternOperandIsRefused = true
		p := syntax.NewParser(src, d)
		p.Parse()
		err := p.Err()
		if err == nil {
			t.Errorf("%s: parsed, want a refusal", src)
			continue
		}
		var se *syntax.Error
		if !asSyntaxError(err, &se) || se.Kind != syntax.ErrUnexpected || se.Token != "(" {
			t.Errorf("%s: %v, want an unexpected `(`", src, err)
		}

		d.GroupOpeningAPatternOperandIsRefused = false
		p = syntax.NewParser(src, d)
		p.Parse()
		if err := p.Err(); err != nil {
			t.Errorf("%s: refused with the flag off: %v", src, err)
		}
	}
}

// What the refusal does not reach. A **word** operand takes a leading `(` in
// every column, an escaped parenthesis is an ordinary character, and a group
// written the way the refusing shell spells its own — `@(` — is read.
func TestAGroupOpeningAPatternOperandLeavesTheRestAlone(t *testing.T) {
	for _, src := range []string{
		`echo "${u:-(a)}"`,
		`echo "${u-(a)}"`,
		`echo "${u:=(a)}"`,
		`echo "${u:+(a)}"`,
		`echo "${v#\(a\)}"`,
		`echo "${v#@(a)}"`,
		`echo "${v#a(b)}"`,
	} {
		d := syntax.Core()
		d.ExtendedPattern = true
		d.GroupOpeningAPatternOperandIsRefused = true
		p := syntax.NewParser(src, d)
		p.Parse()
		if err := p.Err(); err != nil {
			t.Errorf("%s: %v, want it read", src, err)
		}
	}
}

func asSyntaxError(err error, out **syntax.Error) bool {
	se, ok := err.(*syntax.Error)
	if ok {
		*out = se
	}
	return ok
}
