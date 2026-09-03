// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A subscript ends at the `]` that closes it, not at the last one in the word.
//
// `${tags[@]+${tags[@]}}` is the standard way to expand a possibly-empty array
// under `set -u`, and it is where the wild sweep found this: taking the last
// `]` swallowed everything between the two, so the subscript came out as
// `@]+${tags[@` and the operator after it was unreadable.
//
// The simple `${a[@]+x}` always worked, which is why it took a real script
// using the nested form to notice.
func TestASubscriptEndsAtItsOwnBracket(t *testing.T) {
	d := syntax.Core()
	for _, c := range []struct{ name, src, index string }{
		{"a plain subscript", "${a[@]}", "@"},
		{"an operator after one", "${a[@]+x}", "@"},
		{"another expansion in the operand", "${a[@]+${b[@]}}", "@"},
		{"the same name in the operand", "${a[@]+${a[@]}}", "@"},
		{"two levels of it", "${a[@]+${b[@]+${c[@]}}}", "@"},
		// A subscript may hold one of its own, so the match is by depth
		// rather than by the first `]`.
		{"a subscript inside a subscript", "${a[b[0]]}", "b[0]"},
		{"a numeric subscript", "${a[1]-d}", "1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser("echo "+c.src, d)
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			e := onlyParamExpr(t, f)
			if e.Index == nil {
				t.Fatalf("no subscript read from %q", c.src)
			}
			if got := e.Index.Literal(); got != c.index {
				t.Errorf("subscript %q, want %q", got, c.index)
			}
		})
	}
}

// onlyParamExpr digs out the single expansion in `echo ${...}`.
func onlyParamExpr(t *testing.T, f *syntax.File) *syntax.ParamExpr {
	t.Helper()
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("got %T, want a one-command pipeline", f.Stmts[0].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Args) != 2 {
		t.Fatalf("got %T with the wrong shape", pipe.Cmds[0])
	}
	for _, sp := range cmd.Args[1].Spans {
		if sp.Param != nil {
			return sp.Param
		}
	}
	t.Fatal("no parameter expansion in the word")
	return nil
}
