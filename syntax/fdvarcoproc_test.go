// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// `{fd}>f` is a redirection whose descriptor the shell picks — in the core,
// where the braces reach the Redirect; without the flag they are an ordinary
// word, which is how dash reads them.
func TestFdVariableRedirectionIsADialectQuestion(t *testing.T) {
	f, err := Parse(`exec {fd}>out`, Core())
	if err != nil {
		t.Fatal(err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(c.Redirs) != 1 || c.Redirs[0].N == nil || c.Redirs[0].N.Literal() != "{fd}" {
		t.Errorf("core read %+v, want one redirect numbered {fd}", c)
	}
	if len(c.Args) != 1 {
		t.Errorf("core read %d words, want exec alone", len(c.Args))
	}

	f, err = Parse(`exec {fd}>out`, POSIX())
	if err != nil {
		t.Fatal(err)
	}
	c = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(c.Args) != 2 || c.Args[1].Literal() != "{fd}" {
		t.Errorf("POSIX read %+v, want {fd} as an ordinary word", c)
	}

	// The adjacency and the name rule, in the dialect that has it: a space
	// before the operator or a comma in the braces makes a word.
	for _, src := range []string{`echo {fd} >f`, `echo {a,b}>f`, `echo {}>f`, `echo {2x}>f`} {
		g, err := Parse(src, Core())
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		sc := g.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(sc.Args) != 2 {
			t.Errorf("%q read as %d words, want the braces kept as one", src, len(sc.Args))
		}
	}
}

// `coproc` opens a clause only in the dialect that has the flag; elsewhere
// it is a word like any other.
func TestCoprocIsADialectQuestion(t *testing.T) {
	d := Core()
	d.Coproc = true

	f, err := Parse(`coproc cat -u`, d)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
	if !ok {
		t.Fatalf("read %T, want a CoprocClause", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if c.Name != "" {
		t.Errorf("plain form got name %q, want none", c.Name)
	}
	sc, ok := c.Cmd.(*SimpleCmd)
	if !ok || len(sc.Args) != 2 || sc.Args[0].Literal() != "cat" {
		t.Errorf("plain form read %+v, want `cat -u`", c.Cmd)
	}

	f, err = Parse(`coproc UP { cat; }`, d)
	if err != nil {
		t.Fatal(err)
	}
	c = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
	if c.Name != "UP" {
		t.Errorf("named form got %q, want UP", c.Name)
	}
	if _, ok := c.Cmd.(*Group); !ok {
		t.Errorf("named form read %T, want the group", c.Cmd)
	}

	// Without the flag the word opens nothing.
	f, err = Parse(`coproc cat`, Core())
	if err != nil {
		t.Fatal(err)
	}
	sc, ok = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok || len(sc.Args) != 2 {
		t.Errorf("core read %+v, want an ordinary command of two words", f.Stmts[0].Expr)
	}
}

// The printer writes both forms back.
func TestCoprocPrintsBack(t *testing.T) {
	d := Core()
	d.Coproc = true
	for _, src := range []string{`coproc cat`, `coproc UP { cat; }`} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatal(err)
		}
		if got := Print(f); got != src {
			t.Errorf("printed %q, want %q", got, src)
		}
	}
}
