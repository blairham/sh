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
	d.CoprocName = true

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

	// And the name is a second flag over the first. With the word alone the
	// coprocess still runs, and the would-be name is the first word of the
	// command it runs — so a *compound* after it has nothing to open it, and
	// `coproc MY { cat; }` is the `}` that closes nothing.
	nameless := Core()
	nameless.Coproc = true
	f, err = Parse(`coproc MY cat`, nameless)
	if err != nil {
		t.Fatal(err)
	}
	c = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
	if c.Name != "" {
		t.Errorf("nameless dialect got name %q, want none", c.Name)
	}
	sc, ok = c.Cmd.(*SimpleCmd)
	if !ok || len(sc.Args) != 2 || sc.Args[0].Literal() != "MY" {
		t.Errorf("nameless dialect read %+v, want `MY cat` as the command", c.Cmd)
	}
	if _, err := Parse(`coproc MY { cat; }`, nameless); err == nil {
		t.Error("accepted a named compound in a dialect with no name for a coprocess")
	} else if got, want := err.Error(), `1:18: "}" unexpected`; got != want {
		t.Errorf("refused with %q, want %q", got, want)
	}
}

// The printer writes both forms back.
func TestCoprocPrintsBack(t *testing.T) {
	d := Core()
	d.Coproc = true
	d.CoprocName = true
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

// A subscripted name inside the braces is a second flag's, not the first
// one's. The shell that has `{fd}` and arrays and still refuses `{a[1]}` is
// the reason: without a flag of its own, having both features would have
// decided this one.
func TestASubscriptedFdVariableIsItsOwnDialectQuestion(t *testing.T) {
	d := Core()
	d.FdVariableSubscript = true

	f, err := Parse(`exec {a[1]}>out`, d)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(c.Redirs) != 1 || c.Redirs[0].N == nil || c.Redirs[0].N.Literal() != "{a[1]}" {
		t.Errorf("with the flag read %+v, want one redirect numbered {a[1]}", c)
	}
	if len(c.Args) != 1 {
		t.Errorf("with the flag read %d words, want exec alone", len(c.Args))
	}

	// The core has `{fd}` and arrays and still leaves the braces a word,
	// because the three current shells do not agree about this one.
	g, err := Parse(`exec {a[1]}>out`, Core())
	if err != nil {
		t.Fatal(err)
	}
	sc := g.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(sc.Args) != 2 || sc.Args[1].Literal() != "{a[1]}" {
		t.Errorf("core read %+v, want {a[1]} as an ordinary word", sc)
	}

	// What the brackets may hold, and what keeps them clear of brace
	// expansion: a subscript is never empty, never nested, and never carries
	// anything that would have to expand — the token is one literal span, so
	// a `$` in it could only ever mean the character.
	for _, src := range []string{
		`echo {a[]}>f`, `echo {a[b[1]]}>f`, `echo {a[$i]}>f`,
		`echo {a[1],b}>f`, `echo {a[1]} >f`, `echo {[1]}>f`,
	} {
		h, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		w := h.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(w.Args) != 2 {
			t.Errorf("%q read as %d words, want the braces kept as one", src, len(w.Args))
		}
	}

	// And an expression is a subscript, because that is what a subscript is
	// everywhere else in the language.
	for _, src := range []string{`exec {a[i]}>f`, `exec {a[i+1]}>f`, `exec {m[key]}>f`} {
		h, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		w := h.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(w.Redirs) != 1 {
			t.Errorf("%q read as %d redirects, want one", src, len(w.Redirs))
		}
	}
}
