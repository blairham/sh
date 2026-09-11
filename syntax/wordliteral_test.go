// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Word.Literal takes two shortcuts, and these pin that they answer what the
// join answered rather than only that they are fast.
//
// A span's Value is already the string Literal returns, and nearly every word
// in a script is one span — `cmd`, `-x`, `name`, `"$x"`. Joining one of them
// allocated a byte slice and a string to copy something the tree already held:
// 17.5MB of a 154MB interactive startup (#2070).
func TestLiteralJoinsTheSpansHoweverManyThereAre(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		// One span, unquoted and quoted: the shortcut's two common shapes.
		{"a bare word", `echo`, "echo"},
		{"a quoted word", `"echo"`, "echo"},
		{"a single-quoted word", `'echo'`, "echo"},
		// More than one span still joins, with the quotes removed and
		// nothing else done — which is the whole of what Literal promises.
		{"quoted and bare", `a"b"c`, "abc"},
		{"two quotings", `'a'"b"`, "ab"},
		{"empty quotes between", `a""b`, "ab"},
		// An empty word is the zero-span case.
		{"an empty string", `""`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := firstWord(t, tc.src)
			if got := w.Literal(); got != tc.want {
				t.Errorf("Literal() = %q, want %q", got, tc.want)
			}
		})
	}
}

// And nil is the empty string, which several callers rely on: a redirect with
// no descriptor word asks this without checking first.
func TestLiteralOfNoWordIsEmpty(t *testing.T) {
	var w *syntax.Word
	if got := w.Literal(); got != "" {
		t.Errorf("Literal() of a nil word = %q, want empty", got)
	}
}

// firstWord parses `cmd SRC` and returns the argument word.
func firstWord(t *testing.T, src string) *syntax.Word {
	t.Helper()
	p := syntax.NewParser("cmd "+src, syntax.Core())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("%q did not parse to one command", src)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Args) != 2 {
		t.Fatalf("%q did not parse to a command and one argument", src)
	}
	return cmd.Args[1]
}
