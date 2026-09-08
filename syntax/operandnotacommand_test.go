// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// An expansion's operand is not a command position, so the `((` that opens an
// arithmetic command is two ordinary characters in one.
//
// Measured 2026-09-07 and **unanimous** across bash 5.3.15, bash 3.2.57, that
// build invoked as `sh`, dash, ksh93u+ and zsh 5.9.2:
//
//	u=; x=${u:-((a))}; echo "[$x]"      [((a))] in all six
//	u=; y=${u:-$((1+2))}; echo "[$y]"   [3] in all six
//
// so the rule is about where a *command* may begin and not about arithmetic
// being unavailable in an operand. See docs/spec/grammar/parameter-expansion.md.
//
// The assertion is the operand's **text**, because the arithmetic reading is a
// lossy one and text is what it lost: scanArithCommand keeps the expression
// and strips the two opening parens and up to two closing ones, so `((a))`
// came back as the single token `a` and `((#s)a)` as `#s)a` — which is the
// string that reached the matcher as `bad pattern: #s)a` (#1408).
//
// A parse test rather than only a run test: nothing downstream can recover
// characters the lexer did not hand it, so this is the layer the fault lived
// at. The dialect is [syntax.Core] with the operators switched on one at a
// time, so no shell is named here; which shell reads what lives in dialect/.
func TestAnExpansionsOperandIsNotACommandPosition(t *testing.T) {
	d := syntax.Core()
	d.ArithCommand = true
	d.ParamSubstitution = true

	for _, c := range []struct {
		name string
		src  string
		arg  string
		arg2 string
	}{
		// The word operand, which is the unanimous row above.
		{"a word operand", "echo ${u:-((a))}", "((a))", ""},
		// The pattern operands, which is where the same fault was fatal:
		// a leading group is ordinary in the grammar that has groups.
		{"a trim's pattern", "echo ${v#((a))}", "((a))", ""},
		{"a long trim's pattern", "echo ${v##((a))}", "((a))", ""},
		{"a suffix trim's pattern", "echo ${v%((a))}", "((a))", ""},
		{"a flag group inside one", "echo ${v#((#s)a)}", "((#s)a)", ""},
		// Both operands of a replacement, since each is lexed on its own.
		{"a replacement's pattern", "echo ${v/((a))/z}", "((a))", "z"},
		{"a replacement's own text", "echo ${v/z/((a))}", "z", "((a))"},
		// Three parens, so a reading that stripped two closing ones is not
		// the same as one that stripped the balance.
		{"a nested group", "echo ${v#(((a)))}", "(((a)))", ""},
		// Not adjacent, which the `((` rule never reached and which is
		// therefore the control on the control: this always worked.
		{"a spaced pair", "echo ${u:-( (a) )}", "( (a) )", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := paramOf(t, c.src, d)
			if got := argText(e.Arg); got != c.arg {
				t.Errorf("%s: operand = %q, want %q", c.src, got, c.arg)
			}
			if got := argText(e.Arg2); got != c.arg2 {
				t.Errorf("%s: second operand = %q, want %q", c.src, got, c.arg2)
			}
			for _, s := range e.Arg.Spans {
				if s.Kind == syntax.ArithSubst {
					t.Errorf("%s: operand holds an arithmetic span %q — an operand is not a command position",
						c.src, s.Value)
				}
			}
		})
	}
}

// The suspension belongs to the operand and not to arithmetic: `$((` in the
// same position is still an arithmetic substitution, which is the control that
// separates this fix from switching the construct off.
//
// Both operands, and a `((` beside one, because the operand is lexed as a
// whole: a `$((` that stopped being read would leave its text behind as
// literal characters and the row above would still pass.
func TestAnOperandStillTakesAnArithmeticSubstitution(t *testing.T) {
	d := syntax.Core()
	d.ArithCommand = true
	d.ParamSubstitution = true

	for _, c := range []struct {
		src  string
		want string
	}{
		{"echo ${u:-$((1+2))}", "1+2"},
		{"echo ${v#$((1+2))}", "1+2"},
		{"echo ${v/x/$((1+2))}", "1+2"},
		// An arithmetic substitution next to the parentheses that are not
		// one, so the two readings stand in the same operand.
		{"echo ${u:-((a))$((1+2))}", "1+2"},
	} {
		e := paramOf(t, c.src, d)
		w := e.Arg
		if e.Arg2 != nil && len(e.Arg2.Spans) > 0 {
			w = e.Arg2
		}
		var found string
		for _, s := range w.Spans {
			if s.Kind == syntax.ArithSubst {
				found = s.Value
			}
		}
		if found != c.want {
			t.Errorf("%s: arithmetic substitution = %q, want %q", c.src, found, c.want)
		}
	}
}

// An arithmetic command is still one where a command may begin, which is the
// other half of the same control: the operand suspends it, nothing else does.
func TestACommandPositionStillTakesAnArithmeticCommand(t *testing.T) {
	d := syntax.Core()
	d.ArithCommand = true

	for _, src := range []string{
		"((1+2))",
		"x=1; ((x+1)) && echo ran",
		// Inside a substitution, which is a command position of its own and
		// reached through the same operand-lexing path in the parser.
		"echo ${ ((1+2)) ;}",
	} {
		p := syntax.NewParser(src, d)
		f := p.Parse()
		if p.Err() != nil {
			t.Errorf("%s: %v", src, p.Err())
			continue
		}
		if f == nil || len(f.Stmts) == 0 {
			t.Errorf("%s: parsed to nothing", src)
		}
	}
}
