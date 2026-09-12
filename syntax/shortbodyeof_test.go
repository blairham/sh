// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// shortFormDialect is the core plus the two flags the short forms need, so
// that these tests name a grammar rule rather than a shell.
func shortFormDialect() Dialect {
	d := Core()
	d.ShortForm = true
	d.Repeat = true
	d.ArithCommand = true
	d.DoubleBracket = true
	// `{ echo A }` with no terminator before the brace, which is how the
	// short form's own dialect writes a body.
	d.CloseBraceAlwaysReserved = true
	return d
}

// A short-form body that never arrived leaves the input unfinished.
//
// The parse succeeds either way and the tree is the same one: a loop with an
// empty body is what the text means to a program that has no more of it, and
// running it is what a shell does with `for i in 1 2` on the last line of a
// script. What changes is the answer to a second question — did the input run
// out inside the construct — which is the only thing that can tell a prompt to
// ask for the next line rather than to run a loop over nothing (#1298).
func TestAShortBodyThatNeverArrivedLeavesTheInputIncomplete(t *testing.T) {
	for _, src := range []string{
		"for i in 1 2\n",
		"for i in 1 2\n\n",
		"for i (1 2)\n",
		"for i\n",
		"while true\n",
		"until false\n",
		"repeat 2\n",
		// Nested: the inner loop is the one that ran out, and the outer one
		// is still open around it.
		"for i (a b) for j (c d)\n",
		// An `if` whose whole body is missing takes the long form's route to
		// the same answer — `then` never came — and is here so that the two
		// routes are known to agree.
		"if (( 1 ))\n",
		// An arm not written the short way puts the rest of the construct in
		// the long form, so what is missing here is the `fi` — and the same
		// answer comes back, which is what lets a prompt draw its
		// continuation before refusing the line rather than running an arm
		// nobody wrote (#1372).
		"if (( 1 )) { echo A } else\n",
		"if (( 0 )) { echo A } else echo B\n",
		"if (( 1 )) { echo A } elif (( 1 ))\n",
	} {
		p := NewParser(src, shortFormDialect())
		p.Parse()
		if !p.Incomplete() {
			t.Errorf("%q: want Incomplete, so a prompt asks for the body", src)
		}
		if len(p.Open()) == 0 {
			t.Errorf("%q: want the construct that is still open, for the continuation prompt", src)
		}
	}
}

// The same constructs, finished, are not incomplete — which is the half that
// keeps the rule above from being "a short form is never done".
//
// `if (( 1 )) echo A; else echo B` was on this list and is not: the `;` ends
// the whole `if`, so the `else` attaches to nothing and the line is a parse
// error rather than a finished one. TestASeparatedShortArmEndsTheWholeIf has
// it now, with the rest of that rule.
func TestAShortFormWithItsBodyIsComplete(t *testing.T) {
	for _, src := range []string{
		"for i in 1 2; do echo $i; done\n",
		"for i (1 2) echo $i\n",
		"for i (1 2); echo $i\n",
		"while false; do :; done\n",
		"repeat 2 echo R\n",
		"if (( 1 )) echo A\n",
		"echo A\n",
		// A body that is empty because somebody else's token is standing
		// there. The input did not run out — the `)` is in hand — so the
		// construct is finished and the prompt has nothing to ask.
		"( for i (a b) )\n",
	} {
		p := NewParser(src, shortFormDialect())
		p.Parse()
		if err := p.Err(); err != nil {
			t.Errorf("%q: parse failed: %v", src, err)
			continue
		}
		if p.Incomplete() {
			t.Errorf("%q: want complete, got Incomplete with %v open", src, p.Open())
		}
	}
}

// Without the flag there is no short form to be part-way through, so the same
// text is unfinished for the ordinary reason: `do` never came.
func TestWithoutTheShortFormTheHeaderIsStillIncomplete(t *testing.T) {
	for _, src := range []string{"for i in 1 2\n", "while true\n"} {
		p := NewParser(src, Core())
		p.Parse()
		if !p.Incomplete() {
			t.Errorf("%q: want Incomplete without the short form too", src)
		}
	}
}

// Incomplete is the *only* thing that changed: the tree a program gets is
// still a loop with an empty body, and there is still no error.
//
// This is the half that keeps `-c` working. A reader with no more input to
// fetch runs what it has, and `for i in 1 2` on the last line of a script is a
// loop that iterates twice over nothing rather than a syntax error.
func TestTheUnfinishedShortFormIsStillAWholeLoop(t *testing.T) {
	p := NewParser("for i in 1 2\n", shortFormDialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("got %T, want one command", f.Stmts[0].Expr)
	}
	c, ok := pipe.Cmds[0].(*ForClause)
	if !ok {
		t.Fatalf("got %T, want a for loop", pipe.Cmds[0])
	}
	if len(c.Items) != 2 {
		t.Errorf("got %d items, want the two the header names", len(c.Items))
	}
	if len(c.Body) != 0 {
		t.Errorf("got a body of %d statements, want none", len(c.Body))
	}
}
