// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A redirection after a compound command belongs to that command, and the
// C-style loop is no exception.
//
// The failure this pins had no diagnostic in it, because both halves are legal
// on their own: with nowhere on the node to keep a redirection, the parser left
// the operator standing and it became a *statement* of its own — a redirection
// with no command, which truncates the file and redirects nothing. So the
// assertion is the statement count as much as the node's list; either one alone
// would still pass with the redirection in the wrong place.
func TestACStyleForTakesItsRedirection(t *testing.T) {
	for _, src := range []string{
		`for ((i=0;i<2;i++)); do echo "$i"; done >f`,
		`for ((i=0;i<2;i++)) { echo "$i"; } >f`,
		`for ((i=0;i<2;i++)); do read x; done <f`,
	} {
		f, err := Parse(src, Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if len(f.Stmts) != 1 {
			t.Errorf("%q read as %d statements, want the redirection inside the loop's",
				src, len(f.Stmts))
			continue
		}
		c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*ForArithClause)
		if !ok {
			t.Fatalf("%q read as %T, want the loop", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
		}
		if len(c.Redirs) != 1 {
			t.Errorf("%q left the loop %d redirections, want the one written after it",
				src, len(c.Redirs))
		}
	}
}

// And it survives being printed back, which is the other way a redirection
// gets lost: a printer that does not write the suffix produces source that
// means something else and still parses.
func TestAPrintedCStyleForKeepsItsRedirection(t *testing.T) {
	const src = `for ((i=0;i<2;i++)); do echo "$i"; done >f`
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatal(err)
	}
	out := Print(f)
	g, err := Parse(out, Core())
	if err != nil {
		t.Fatalf("reparse %q: %v", out, err)
	}
	if len(g.Stmts) != 1 {
		t.Fatalf("printed %q, which reads as %d statements", out, len(g.Stmts))
	}
	c := g.Stmts[0].Expr.(*Pipeline).Cmds[0].(*ForArithClause)
	if len(c.Redirs) != 1 {
		t.Errorf("printed %q, which lost the redirection", out)
	}
}
