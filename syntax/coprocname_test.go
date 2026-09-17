// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The word standing where a coprocess name belongs is not required to be a
// name: the shell with the construct takes it and judges what it expands to
// when the clause runs. See parseCoproc, where the measurement is.
//
// The cost of refusing it here was not the clause — it was **everything after
// it**. A refusal while parsing is a syntax error, which ends the script, so
// one such line took the rest of the file with it.
func TestACoprocessNameIsAnyWordTheGrammarCanTake(t *testing.T) {
	t.Parallel()
	d := Core()
	d.Coproc = true
	d.CoprocName = true

	for _, src := range []string{
		`coproc @ { cat; }`,
		`coproc a-b { cat; }`,
		`coproc 1x { cat; }`,
		`coproc a=b { cat; }`,
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
		if !ok {
			t.Fatalf("%q read %T, want a CoprocClause", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
		}
		// The word, not the text: it has to be expanded before anyone can
		// say whether it is a name.
		if c.NameWord == nil {
			t.Errorf("%q kept no name word, got Name %q", src, c.Name)
		}
		if c.Name != "" {
			t.Errorf("%q put %q in Name, want the word instead", src, c.Name)
		}
		if _, ok := c.Cmd.(*Group); !ok {
			t.Errorf("%q read %T as the command, want the group", src, c.Cmd)
		}
		// And it reads back as it was written, which is the only print that
		// names the same coprocess.
		if got := Print(f); got != src {
			t.Errorf("printed %q, want %q", got, src)
		}
	}

	// A word carrying an expansion is kept for the same reason and is not a
	// refusal at all: `coproc $v { … }` publishes under whatever `$v` is.
	f, err := Parse(`coproc $v { cat; }`, d)
	if err != nil {
		t.Fatal(err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
	if c.NameWord == nil || c.Name != "" {
		t.Errorf("an expanded name got Name %q and word %v, want the word alone", c.Name, c.NameWord)
	}
	if got, want := Print(f), `coproc $v { cat; }`; got != want {
		t.Errorf("printed %q, want %q", got, want)
	}

	// A bare name still lands in Name, because that is nearly every
	// coprocess anyone writes and a word there would make every reader
	// expand before it could ask what the name was.
	f, err = Parse(`coproc UP { cat; }`, d)
	if err != nil {
		t.Fatal(err)
	}
	c = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
	if c.Name != "UP" || c.NameWord != nil {
		t.Errorf("a bare name got Name %q and word %v, want UP and no word", c.Name, c.NameWord)
	}
}

// The two words that may **not** be read as a name, because reading them as
// one would swallow the command or the list.
//
// `coproc { … }` is the nameless form and the braces are its command; `!` is
// refused where a name belongs, measured on bash 5.3.20. Without these the
// relaxation above would have taken the `{` of every nameless compound as the
// name and then read the body as a simple command.
func TestWhatCannotBeACoprocessName(t *testing.T) {
	t.Parallel()
	d := Core()
	d.Coproc = true
	d.CoprocName = true

	for _, src := range []string{
		`coproc { cat; }`,
		`coproc if true; then cat; fi`,
		`coproc while false; do cat; done`,
		`coproc ( cat )`,
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CoprocClause)
		if c.Name != "" || c.NameWord != nil {
			t.Errorf("%q took a name: %q / %v", src, c.Name, c.NameWord)
		}
		if got := Print(f); got != src {
			t.Errorf("printed %q, want %q", got, src)
		}
	}

	if _, err := Parse(`coproc ! { cat; }`, d); err == nil {
		t.Error("read `!` as a coprocess name, want the refusal bash makes")
	}
}
