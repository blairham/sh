// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// One digit before a redirection operator is a descriptor number everywhere.
// A second one is a dialect question: with the flag the whole run of digits is
// the number, and without it they are an ordinary word and the operator is a
// redirection with no number of its own.
func TestATwoDigitDescriptorNumberIsADialectQuestion(t *testing.T) {
	d := Core()
	d.MultiDigitFdNumber = true

	f, err := Parse(`exec 10>out`, d)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(c.Redirs) != 1 || c.Redirs[0].N == nil || c.Redirs[0].N.Literal() != "10" {
		t.Errorf("with the flag read %+v, want one redirect numbered 10", c)
	}
	if len(c.Args) != 1 {
		t.Errorf("with the flag read %d words, want exec alone", len(c.Args))
	}

	f, err = Parse(`exec 10>out`, Core())
	if err != nil {
		t.Fatal(err)
	}
	c = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(c.Args) != 2 || c.Args[1].Literal() != "10" {
		t.Errorf("the core read %+v, want 10 as an ordinary word", c)
	}
	if len(c.Redirs) != 1 || c.Redirs[0].N != nil {
		t.Errorf("the core read %+v, want a redirection with no number", c)
	}
}

// The single digit is not the flag's to take away, in either dialect, and
// neither is the adjacency rule that decides whether it is a number at all.
func TestOneDigitIsADescriptorNumberWhicheverWayTheFlagIsSet(t *testing.T) {
	for _, multi := range []bool{false, true} {
		d := Core()
		d.MultiDigitFdNumber = multi

		f, err := Parse(`echo 1>out`, d)
		if err != nil {
			t.Fatal(err)
		}
		c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(c.Redirs) != 1 || c.Redirs[0].N == nil || c.Redirs[0].N.Literal() != "1" {
			t.Errorf("multi=%v read %+v, want one redirect numbered 1", multi, c)
		}

		// A space before the operator makes a word of it, whatever the width.
		g, err := Parse(`echo 1 >out`, d)
		if err != nil {
			t.Fatal(err)
		}
		sc := g.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(sc.Args) != 2 || sc.Args[1].Literal() != "1" {
			t.Errorf("multi=%v read %+v, want the 1 as an argument", multi, sc)
		}
	}
}
