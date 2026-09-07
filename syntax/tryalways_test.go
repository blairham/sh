// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The try-always block, `{ … } always { … }` (#1216). The flag is named here
// and the shell that sets it is not.
//
// Measured 2026-09-07 against zsh 5.9.2, the only panel shell that has the
// construct: the other five report the keyword as an unexpected token where it
// stands. Every row below is asserted with the flag *off* as well, because a
// production that parses under the core would take the construct away from
// nobody and hand it to everybody.

// tryAlways is the core plus the try-always block.
//
// CloseBraceAlwaysReserved travels with it in the one preset that has either,
// and it is what lets `{ echo t } always { echo a }` close each half without a
// terminator — the third of the three spellings the issue measured, and the
// one a rule about separators rather than about the keyword would have broken.
func tryAlways() Dialect {
	d := Core()
	d.TryAlways = true
	d.CloseBraceAlwaysReserved = true
	return d
}

func TestABraceGroupMayTakeACleanupHalf(t *testing.T) {
	on, off := tryAlways(), Core()
	for _, src := range []string{
		`{ echo t; } always { echo a; }`,
		// The `}` and the keyword on one line with no separator between them
		// is the shape every real script uses; the bodies are ordinary lists.
		"{\n  echo t\n} always {\n  echo a\n}\n",
		// Without a terminator before either `}`, which needs the brace rule
		// beside this flag rather than a change to this one.
		`{ echo t } always { echo a }`,
		// Nested, in each half in turn.
		`{ { echo t; } always { echo a; }; } always { echo b; }`,
		`{ echo t; } always { { echo a; } always { echo b; }; }`,
		// Either half may be empty where the dialect allows an empty body,
		// and the construct is not what decides that.
		`{ :; } always { :; }`,
		// A redirection after the second half belongs to the construct.
		`{ echo t; } always { echo a; } > /dev/null`,
		// And it is a compound command, so it stands wherever one does.
		`f() { { echo t; } always { echo a; }; }`,
		`for i in a; do { echo t; } always { echo a; }; done`,
		`case x in x) { echo t; } always { echo a; };; esac`,
		`echo x | { cat; } always { echo a; }`,
	} {
		mustParse(t, src, on, "a brace group takes a cleanup half")
		mustFail(t, src, off, "and it does not, without the flag")
	}
}

// The keyword is **positional**: it is read only where a brace group has just
// closed, and it is an ordinary word everywhere else.
//
// This is the whole of the grammar, and it is what says the lexer must not
// learn the word. All six panel columns run `always`, define a function called
// it, print it as an argument and assign it as a value — so putting it in
// stopWords or reservedWords would take four working spellings away from every
// dialect to add one to a single dialect.
func TestTheCleanupKeywordIsAnOrdinaryWordEverywhereElse(t *testing.T) {
	on := tryAlways()
	for _, src := range []string{
		`always`,
		`always arg`,
		`always() { :; }`,
		`always() { :; }; always`,
		`echo always`,
		`x=always; echo $x`,
		// A separator before it is the same as any other word before a
		// command: the brace group has closed and ended the command with it.
		`{ echo t; }; always`,
		`{ echo t; } | always`,
		`{ echo t; } && always`,
	} {
		mustParse(t, src, on, "the keyword is an ordinary word away from a closed brace")
		mustParse(t, src, Core(), "and it is one under the core too")
	}
}

// The shapes the flag does *not* add, each of which is a parse error in the
// shell that has the construct. They are what make this a production hanging
// off the brace group rather than "a word that may follow anything".
func TestTheCleanupHalfHasABoundary(t *testing.T) {
	on := tryAlways()
	for _, src := range []string{
		// A newline is a separator wherever a `;` is.
		"{ echo t; }\nalways { echo a; }\n",
		// Quoting or escaping the word takes the keyword away, which falls out
		// of reading it as an unquoted literal rather than being written twice.
		`{ echo t; } "always" { echo a; }`,
		`{ echo t; } 'always' { echo a; }`,
		`{ echo t; } \always { echo a; }`,
		`{ echo t; } al""ways { echo a; }`,
		// The second half has to be a brace group.
		`{ echo t; } always echo a`,
		`{ echo t; } always ( echo a )`,
		`{ echo t; } always`,
		// And there is exactly one of them.
		`{ echo t; } always { echo a; } always { echo b; }`,
		// A redirection between the halves ends the first one, so the keyword
		// is then a word where a command belongs.
		`{ echo t; } > /dev/null always { echo a; }`,
		// No other compound command takes it, a loop's brace body included —
		// which is why the loop reads its group directly rather than through
		// the command dispatch.
		`if true; then echo t; fi always { echo a; }`,
		`( echo t ) always { echo a; }`,
		`for i in a; do echo $i; done always { echo a; }`,
		`while false; do echo t; done always { echo a; }`,
		`case x in x) :;; esac always { echo a; }`,
		`for i in a; { echo $i; } always { echo a; }`,
		// Nor a function definition's body, named or nameless.
		`f() { echo t; } always { echo a; }`,
		`function f { echo t; } always { echo a; }`,
	} {
		mustFail(t, src, on, "the cleanup half is read only where a brace group has just closed")
	}
	// A nameless function is the one of those that needs both flags to be
	// reachable at all: without AnonymousFunction the `()` is a different
	// refusal and the row would pass for the wrong reason.
	anon := tryAlways()
	anon.AnonymousFunction = true
	for _, src := range []string{
		`() { echo t; } always { echo a; }`,
		`function { echo t; } always { echo a; }`,
	} {
		mustFail(t, src, anon, "a nameless function's body is a body too")
	}
	mustParse(t, `() { echo t; }`, anon, "and the definition itself still parses")
}

// A try-always block closes itself, so a header that has ended may be followed
// straight by its body where the dialect has short forms.
//
// Measured 2026-09-07: `if { true; } always { :; } { echo A; }; echo after`
// prints A and after in zsh 5.9.2. The trailing statement is load-bearing —
// a command string whose last byte is the `}` of a short body is a parse error
// there whatever precedes it, so a probe without one measures the route rather
// than the grammar.
func TestATryAlwaysBlockEndsAHeaderThatWouldNotEndItself(t *testing.T) {
	on := tryAlways()
	on.ShortForm = true
	for _, src := range []string{
		"if { true; } always { :; } { echo A; }; echo after\n",
		"if { true; } always { :; } echo A; echo after\n",
		"while { false; } always { :; } { echo A; }; echo after\n",
	} {
		mustParse(t, src, on, "a try-always block ends a header")
	}
	// The boundary the same flag draws elsewhere is untouched: a *word* still
	// does not end a header, so this is the construct closing and not the
	// short-form rule having been widened.
	mustFail(t, "if true { echo A; }\n", on, "a word does not end a header")
}

// The tree the parser builds, because "it parses" is not the claim: the two
// halves have to be separable and the redirections have to be the
// construct's.
func TestATryAlwaysBlockIsOneCommandWithTwoLists(t *testing.T) {
	f, err := Parse(`{ echo one; echo two; } always { echo a; } > /dev/null`, tryAlways())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("parsed to %d statements, want 1", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("parsed to %T, want one command", f.Stmts[0].Expr)
	}
	tc, ok := pipe.Cmds[0].(*TryClause)
	if !ok {
		t.Fatalf("parsed to %T, want a *TryClause", pipe.Cmds[0])
	}
	if len(tc.Try) != 2 {
		t.Errorf("Try has %d statements, want 2", len(tc.Try))
	}
	if len(tc.Always) != 1 {
		t.Errorf("Always has %d statements, want 1", len(tc.Always))
	}
	if len(tc.Redirs) != 1 {
		t.Fatalf("Redirs has %d entries, want the one written after the second half", len(tc.Redirs))
	}
	// Pos and End span the whole construct, which is what a diagnostic and the
	// printer both read.
	if !tc.End().After(tc.Pos()) {
		t.Errorf("End %v is not after Pos %v", tc.End(), tc.Pos())
	}
	if got := PrintCommand(tc); got != `{ echo one; echo two; } always { echo a; } > /dev/null` {
		t.Errorf("printed %q, want the construct back", got)
	}
}

// What is printed parses again, and to the same shape. The printer is the only
// thing that has to know how the construct is spelled, so a node it renders as
// two unrelated groups would be a silently lossy round trip.
func TestATryAlwaysBlockSurvivesBeingPrinted(t *testing.T) {
	for _, src := range []string{
		`{ echo t; } always { echo a; }`,
		`{ { echo t; } always { echo a; }; } always { echo b; }`,
		`{ echo t; } always { echo a; } > /dev/null`,
		`f() { { echo t; } always { echo a; }; }`,
	} {
		f, err := Parse(src, tryAlways())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		printed := Print(f)
		again, err := Parse(printed, tryAlways())
		if err != nil {
			t.Fatalf("printed %q, which does not parse: %v", printed, err)
		}
		if second := Print(again); second != printed {
			t.Errorf("%q printed %q and then %q", src, printed, second)
		}
	}
}
