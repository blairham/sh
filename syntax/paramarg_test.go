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
		// A `#` inside an operand starts no comment, so what follows is a
		// word like any other and the blank in front of it is the gap
		// between two of them.
		//
		// It used to arrive as the single span `" #b"`, because the lexer
		// *did* read it as a comment and stepped over the rest and this
		// offset repair put the skipped run back. The text was right and
		// nothing inside it was: a span put back this way is raw, so the
		// quoting of anything behind the `#` was gone. See
		// TestAnExpansionsOperandHasNoComment (#2074).
		{"a hash", `${u:-a #b}`, []string{"a", " ", "#b"}},
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
