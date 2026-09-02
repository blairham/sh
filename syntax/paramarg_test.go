// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Everything up to the closing brace belongs to an operand, blanks included.
//
// The operand is re-lexed, and a lexer reading a *command* is right to step
// over the blank between two words — an operand is not a command. Dropping it
// made `${GREP:-grep -E}` come out as `grep-E`, which is a command nobody has,
// and the shell said nothing.
func TestAnOperandKeepsWhatIsBetweenItsWords(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"one space", `${u:-a b}`, []string{"a", " ", "b"}},
		// A run is one run and not one space, which is what says the text
		// was kept rather than rebuilt from the tokens.
		{"a run of them", `${u:-a   b}`, []string{"a", "   ", "b"}},
		{"a tab", "${u:-a\tb}", []string{"a", "\t", "b"}},
		// A `#` inside an operand starts no comment, so what follows is
		// text. It arrives as one span rather than two because the lexer
		// *did* read it as a comment and stepped over the rest — which is
		// exactly the text this puts back, and why putting it back by offset
		// is worth more than reconstructing a separator would be.
		{"a hash", `${u:-a #b}`, []string{"a", " #b"}},
		{"leading and trailing", `${u:- a }`, []string{" ", "a", " "}},
		{"nothing between", `${u:-ab}`, []string{"ab"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arg := parseOperand(t, tc.src)
			var got []string
			for _, s := range arg.Spans {
				got = append(got, s.Value)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("spans = %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("span %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// parseOperand returns the operand word of the first `${…}` in src.
func parseOperand(t *testing.T, src string) *syntax.Word {
	t.Helper()
	f, err := syntax.Parse("echo "+src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	for _, w := range sc.Args {
		for _, s := range w.Spans {
			if s.Param != nil && s.Param.Arg != nil {
				return s.Param.Arg
			}
		}
	}
	t.Fatalf("no operand in %q", src)
	return nil
}
