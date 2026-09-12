// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect says about a word spelled like an operator it does not
// have, in an expression long enough to be parsed rather than counted
// (#1290).
//
// `-Q` is used rather than any real letter on purpose: no dialect has it, so
// the row is about the *spelling* and not about one missing operator.
//
// Measured 2026-09-12:
//
//	dash         dash: 1: [: -Q: unexpected operator          2
//	bash 5.3.15  bash: line 1: [: too many arguments          2
//	bash-as-sh   sh: line 1: [: too many arguments            2
//	bash 3.2.57  bash: line 0: [: -Q: unary operator expected 2
//	ksh93u+      ksh: [: x: unknown operator                  2
//	zsh 5.9.2    zsh:[:1: unknown condition: -Q               2
//
// Three answers rather than two, which is why the dialect field is an
// enumeration: dash and ksh93 decline to read the word as an operator and
// complain that the primary is two operands — naming the first and the second
// of them respectively — zsh reads it as an operator and says it has never
// heard of it, and bash reports the argument count. bash's answer is the one
// that hides which word was wrong, and it is still bash's: a fix that names
// the operator everywhere would regress it.
func TestEachDialectMeetsAnUnknownOperatorInALongExpression(t *testing.T) {
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"bash", "bash: line 1: [: too many arguments\nst=2\n"},
		{"dash", "dash: 1: [: -Q: unexpected operator\nst=2\n"},
		{"ksh", "ksh: [: x: unknown operator\nst=2\n"},
		{"zsh", "zsh:[:1: unknown condition: -Q\nst=2\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			out, _, err := p.Combined(t, dialecttest.Base{}, `x=1; [ -Q x -a -n x ]; echo "st=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}

// The same word in the two short forms, which never reach the grammar: two
// arguments go straight to the unary evaluation and three to the binary one.
//
// They are here because they are what the long form was measured against —
// the complaint changing between `[ -Q x ]` and `[ -Q x -a -n x ]` was the
// defect — so a change that fixes one by breaking the other has to fail.
func TestTheShortFormsNameTheOperatorToo(t *testing.T) {
	for _, c := range []struct {
		dialect string
		two     string
		three   string
	}{
		{"bash", "bash: line 1: [: -Q: unary operator expected\n", "bash: line 1: [: -Q: binary operator expected\n"},
		{"dash", "dash: 1: [: -Q: unexpected operator\n", "dash: 1: [: x: unexpected operator\n"},
		{"ksh", "ksh: [: -Q: unknown operator\n", "ksh: [: -Q: unknown operator\n"},
		{"zsh", "zsh:[:1: unknown condition: -Q\n", "zsh:1: condition expected: -Q\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			out, _, err := p.Combined(t, dialecttest.Base{}, `x=1; [ -Q x ]`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.two {
				t.Errorf("two words said %q, want %q", out, c.two)
			}
			out, _, err = p.Combined(t, dialecttest.Base{}, `x=1; [ x -Q x ]`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.three {
				t.Errorf("three words said %q, want %q", out, c.three)
			}
		})
	}
}

// A word spelled like an operator and standing *alone* as a primary is a
// non-empty string and is true, in every column — `[ -Q -a -n x ]` succeeds
// everywhere — so the complaint needs a word after it that the grammar cannot
// take. This is the row that keeps the new refusal from swallowing a working
// expression.
func TestAnOperatorSpellingAloneIsStillAString(t *testing.T) {
	for name := range presets {
		t.Run(name, func(t *testing.T) {
			p := presets[name]
			out, _, err := p.Combined(t, dialecttest.Base{}, `x=1; [ -Q -a -n x ]; echo "st=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != "st=0\n" {
				t.Errorf("said %q, want %q", out, "st=0\n")
			}
		})
	}
}
