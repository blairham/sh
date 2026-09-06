// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// disowning is the core with the one flag this file is about.
func disowning() Dialect {
	d := Core()
	d.BackgroundAndDisown = true
	return d
}

// TestABackgroundedStatementMayBeDisowned — `&!` and `&|` end a statement the
// way `&` does and additionally let go of the job. Both spellings are one
// operator: every assertion here holds for each.
func TestABackgroundedStatementMayBeDisowned(t *testing.T) {
	on := disowning()
	for _, op := range []string{"&!", "&|"} {
		f, err := Parse("echo hi "+op+"\necho done", on)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if len(f.Stmts) != 2 {
			t.Fatalf("%s: %d statements, want 2", op, len(f.Stmts))
		}
		if !f.Stmts[0].Background || !f.Stmts[0].Disown {
			t.Errorf("%s: background=%v disown=%v, want both", op,
				f.Stmts[0].Background, f.Stmts[0].Disown)
		}
		// The statement after it is an ordinary one, which is what says the
		// operator terminated rather than joined.
		if f.Stmts[1].Background || f.Stmts[1].Disown {
			t.Errorf("%s: the next statement was backgrounded too", op)
		}
	}
	// A plain `&` is background and *not* disown, which is the pair this
	// change could have collapsed.
	f, err := Parse("echo hi &", on)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Stmts[0].Background || f.Stmts[0].Disown {
		t.Errorf("`&`: background=%v disown=%v, want background alone",
			f.Stmts[0].Background, f.Stmts[0].Disown)
	}
	// It belongs to the *statement* and not to the command, the same way `&`
	// does: `a && b &!` lets go of the whole and-or.
	f, err = Parse("true && false &!", on)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 1 || !f.Stmts[0].Disown {
		t.Errorf("an and-or was not disowned as one statement: %d statements", len(f.Stmts))
	}
	if _, isChain := f.Stmts[0].Expr.(*BinaryExpr); !isChain {
		t.Errorf("the whole and-or should be the statement's expression, got %T", f.Stmts[0].Expr)
	}
}

// TestWithoutTheFlagTheOperatorsAreNotThere — the `&` still ends the
// statement and what follows is read as it always was, which is the shape
// every other operator flag has: a dialect that has not got `&!` does not get
// a syntax error for the `&`, it gets `&` and then a `!`.
func TestWithoutTheFlagTheOperatorsAreNotThere(t *testing.T) {
	off := Core()
	if _, err := Parse("echo hi &!\necho done", off); err == nil {
		t.Error("`echo hi &!` parsed without the flag")
	}
	if _, err := Parse("echo hi &|\necho done", off); err == nil {
		t.Error("`echo hi &|` parsed without the flag")
	}
	// And the `&` on its own is untouched by the flag being on: the operator
	// table takes the longest match, so this is the case that says `&!` did
	// not swallow a `&` in front of an unrelated `!`.
	f, err := Parse("echo hi & ! false", disowning())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("%d statements, want 2", len(f.Stmts))
	}
	if f.Stmts[0].Disown {
		t.Error("`& ! false` was read as a disowning")
	}
}

// TestADisownedStatementPrintsBack — the printer's promise is that printed
// source means the same thing, and a `&` where the source wrote `&!` is a
// different program: the job would be in the table.
//
// One spelling of the two goes back. `&!` and `&|` parse to the same tree, so
// there is nothing in it to choose between them.
func TestADisownedStatementPrintsBack(t *testing.T) {
	on := disowning()
	for _, tc := range []struct{ src, want string }{
		{"echo hi &!", "echo hi &!"},
		{"echo hi &|", "echo hi &!"},
		{"echo hi &", "echo hi &"},
		{"true && false &!", "true && false &!"},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		got := Print(f)
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
		}
		back, err := Parse(got, on)
		if err != nil {
			t.Errorf("%s: printed %q, which does not parse: %v", tc.src, got, err)
			continue
		}
		if back.Stmts[0].Disown != f.Stmts[0].Disown {
			t.Errorf("%s: printed %q, whose disowning is %v rather than %v",
				tc.src, got, back.Stmts[0].Disown, f.Stmts[0].Disown)
		}
	}
}
