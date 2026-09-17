// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// arithSpanIn returns the first arithmetic span in the arguments of the only
// command of src.
func arithSpanIn(t *testing.T, src string, d syntax.Dialect) syntax.Span {
	t.Helper()
	cmd, ok := onlyCommand(t, src, d).(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("%q is not a simple command", src)
	}
	for _, w := range cmd.Args {
		for _, sp := range w.Spans {
			if sp.Kind == syntax.ArithSubst {
				return sp
			}
		}
	}
	t.Fatalf("%q holds no arithmetic span", src)
	return syntax.Span{}
}

// A backslash-newline inside an arithmetic expression is a line continuation,
// removed before the expression is read, wherever it stands in it — inside a
// number, between the two characters of an operator, inside a name.
//
// Measured 2026-09-16 from script files, and unanimous in bash 5.3, bash 3.2,
// zsh 5.9.2, ksh93u+ and dash for every shape each has: `$(( 1\⏎2 + 1 ))` is
// 13, `$(( 1 <\⏎< 3 ))` is 8 and `ab\⏎c` is the name abc. The scanner stepped
// over the pair and kept it, so the expression handed on still held a
// backslash and a newline and every dialect stopped the script there (#3442).
//
// Asserted on the text and on the tree both: the text is what a diagnostic
// quotes and what an expression with an expansion in it is read from later,
// and the tree is what says the continuation no longer stands in the reading.
func TestALineContinuationIsRemovedFromAnArithmeticExpansion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src, value string
		parsed           bool // the expression reads without an expansion
	}{
		{"inside a number", "echo $(( 1\\\n2 + 1 ))", " 12 + 1 ", true},
		{"between an operator's characters", "echo $(( 1 <\\\n< 3 ))", " 1 << 3 ", true},
		{"between a power's stars", "echo $(( 3 *\\\n* 2 ))", " 3 ** 2 ", true},
		{"inside a name", "echo $(( ab\\\nc + 1 ))", " abc + 1 ", true},
		{"right after the opener", "echo $((\\\n 1 + 2 ))", " 1 + 2 ", true},
		{"before a closing group paren", "echo $(( (2\\\n) ))", " (2) ", true},
		{"inside a double-quoted word", "echo \"$(( 1\\\n+1 ))\"", " 1+1 ", true},
		{"two of them", "echo $(( 1\\\n2 +\\\n3 ))", " 12 +3 ", true},

		// What is not the expression's own continuation stays where it is.
		// A backslash that is itself escaped makes no continuation: all five
		// shells call `$(( 1 +\\⏎2 ))` an arithmetic syntax error.
		{"an escaped backslash", "echo $(( 1 +\\\\\n2 ))", " 1 +\\\\\n2 ", false},
		// A single-quoted part is literal, whatever the dialect then makes
		// of the quote.
		{"a single-quoted part", "echo $(( '1\\\n2' ))", " '1\\\n2' ", false},
		// A command substitution's text is a program, read by its own rules
		// when it runs, and the quotes in it are its own: `$(( $(printf %s
		// 'a\⏎b' | wc -c) ))` counts four bytes in all five.
		{"a single quote inside a command substitution", "echo $(( $(printf %s 'a\\\nb' | wc -c) ))", " $(printf %s 'a\\\nb' | wc -c) ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := arithSpanIn(t, tc.src, syntax.Core())
			if sp.Value != tc.value {
				t.Errorf("%q holds %q, want %q", tc.src, sp.Value, tc.value)
			}
			if got := sp.Arith != nil; got != tc.parsed {
				t.Errorf("%q read into a tree = %v, want %v", tc.src, got, tc.parsed)
			}
		})
	}
}

// A double-quoted part of an expression is a double-quoted string, and a
// continuation inside one is removed the way it is in any other: `$((
// "1\⏎2" + 1 ))` is 13 in bash 5.3, zsh and ksh93, the three that read
// through the quote at all. Asked of the dialect that removes the quotes,
// because in the one that refuses them the text is refused either way.
func TestALineContinuationIsRemovedInsideADoubleQuotedPartOfAnExpression(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.ArithDoubleQuote = syntax.ArithDoubleQuoteRemoved
	sp := arithSpanIn(t, "echo $(( \"1\\\n2\" + 1 ))", d)
	if want := ` "12" + 1 `; sp.Value != want {
		t.Errorf("value = %q, want %q", sp.Value, want)
	}
	if sp.Arith == nil {
		t.Error("the expression did not read")
	}
}

// The two other spellings of the expansion scan their own text and remove
// the continuation the same way.
func TestALineContinuationIsRemovedFromTheOtherArithmeticSpellings(t *testing.T) {
	t.Parallel()
	bracket := syntax.Core()
	bracket.DollarBracketArith = true
	braced := syntax.Core()
	braced.BracedArithmeticExpansion = true
	for _, tc := range []struct {
		name, src, value string
		d                syntax.Dialect
	}{
		// bash 5.3, bash 3.2 and zsh: `$[ 1\⏎+2 ]` is 3.
		{"the bracket spelling", "echo $[ 1\\\n+2 ]", " 1+2 ", bracket},
		// ksh93u+: `${((\⏎2+3))}` is 5.
		{"the braced spelling", "echo ${((\\\n2+3))}", "2+3", braced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := arithSpanIn(t, tc.src, tc.d)
			if sp.Value != tc.value {
				t.Errorf("%q holds %q, want %q", tc.src, sp.Value, tc.value)
			}
			if sp.Arith == nil {
				t.Errorf("%q did not read", tc.src)
			}
		})
	}
}

// The arithmetic command and the C-style `for` header are the same
// expression reached through a scanner of their own. bash 5.3, bash 3.2, zsh
// and ksh93 all run `(( 2\⏎+2 ))` as true and `for ((i=0; i<1\⏎; i++))` once.
func TestALineContinuationIsRemovedFromAnArithmeticCommand(t *testing.T) {
	t.Parallel()
	t.Run("the command", func(t *testing.T) {
		src := "(( 2\\\n+2 ))"
		c, ok := onlyCommand(t, src, syntax.Core()).(*syntax.ArithCmdClause)
		if !ok {
			t.Fatalf("%q is not an arithmetic command", src)
		}
		if want := " 2+2 "; c.Expr != want {
			t.Errorf("expression = %q, want %q", c.Expr, want)
		}
		if c.Parsed == nil {
			t.Error("the expression did not read")
		}
	})
	t.Run("the for header", func(t *testing.T) {
		src := "for ((i=0; i<1\\\n; i\\\n++)); do :; done"
		c, ok := onlyCommand(t, src, syntax.Core()).(*syntax.ForArithClause)
		if !ok {
			t.Fatalf("%q is not a C-style for", src)
		}
		if c.CondText != "i<1" || c.PostText != "i++" {
			t.Errorf("parts = %q, %q, want %q, %q", c.CondText, c.PostText, "i<1", "i++")
		}
		if c.Cond == nil || c.Post == nil {
			t.Error("a part did not read")
		}
	})
}
