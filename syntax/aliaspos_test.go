// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A word that came from an alias is positioned where the alias was *used*.
//
// Measured: bash, dash and ksh93 all report a command from an alias at the
// line the alias word was written on, and `$LINENO` inside a body reads the
// same. That is what makes a token-level splice honest — every position still
// points into the real input, so nothing downstream needs a mapping.
func TestAnExpandedWordIsPositionedWhereItWasUsed(t *testing.T) {
	const src = "echo one\nbad\n"
	p := syntax.NewParser(src, syntax.Core())
	p.Aliases = table("bad", "nosuchcmd --flag")
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(f.Stmts))
	}
	pipe, ok := f.Stmts[1].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("got %T, want a one-command pipeline", f.Stmts[1].Expr)
	}
	cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("got %T, want a simple command", pipe.Cmds[0])
	}
	if len(cmd.Args) != 2 {
		t.Fatalf("got %d words, want 2", len(cmd.Args))
	}
	// Both words came from the value, and both are on line 2 where `bad` is —
	// not line 1, and not some line inside a body that has no line.
	for i, w := range cmd.Args {
		if got := w.Pos().Line; got != 2 {
			t.Errorf("word %d at line %d, want 2", i, got)
		}
	}
	if got := cmd.Start.Line; got != 2 {
		t.Errorf("command starts at line %d, want 2", got)
	}
}
