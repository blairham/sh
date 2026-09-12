// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// withCoprocPipeOperator is Core plus the one flag under test. A core test
// names a flag rather than a shell.
func withCoprocPipeOperator() Dialect {
	d := Core()
	d.CoprocPipeOperator = true
	return d
}

// `|&` is a **command terminator** under this flag, not a pipe — the other
// reading of the same two bytes.
//
// The measurement that separates them is what may *follow* the operator
// rather than what the command prints, because the output does not
// discriminate: `echo one |& echo two` prints `two` under both readings.
// Measured on ksh93u+ 2012-08-01, 2026-09-07, `-n` over a script file under
// `env -i`:
//
//	probe                    bash 5.3    ksh93     zsh
//	echo one | ; echo two    error `;`   error `;` accepts
//	echo one |& ; echo two   error `;`   accepts   accepts
//	echo one & ; echo two    error `;`   accepts   accepts
//
// A `|` needs a command after it and ksh93's `|&` does not, exactly as a bare
// `&` does not. zsh accepts all three because it is lenient about `;` after
// any control operator, so the ksh column alone decides it.
//
// The `;` half of that landed separately, as #1142 — a `;` written where a
// command belongs — so the discriminating pair is now reachable here and is
// asserted below rather than only described: `a |& ; b` parses and `a | ; b`
// does not, in the same dialect, which is the whole of what tells the two
// readings apart.
func TestTheCoprocessOperatorTerminatesRatherThanJoins(t *testing.T) {
	d := withCoprocPipeOperator()
	mustParse(t, `cat |&`, d, "an operator that needs no command after it")
	f := mustFile(t, `cat |&`, d)
	if n := len(f.Stmts); n != 1 {
		t.Fatalf("statements = %d, want 1", n)
	}
	st := f.Stmts[0]
	if !st.Coprocess || !st.Background {
		t.Errorf("statement = %+v, want a backgrounded coprocess", st)
	}
	// A bar in the same place still demands one, which is what says the flag
	// did not simply make every bar optional.
	if _, err := Parse(`cat |`, d); err == nil {
		t.Error("`cat |` parsed, want a bar with no command refused")
	}
	// And two commands either side of it are two statements rather than a
	// pipeline: `echo one |& echo two` prints only `two`, `one` having gone
	// into the coprocess pipe.
	f = mustFile(t, `echo one |& echo two`, d)
	if n := len(f.Stmts); n != 2 {
		t.Fatalf("statements = %d, want 2", n)
	}
	if !f.Stmts[0].Coprocess || f.Stmts[1].Coprocess {
		t.Errorf("statements = %+v %+v, want only the first a coprocess", f.Stmts[0], f.Stmts[1])
	}
	// A pipe after it is refused, because the pipeline is over: ksh93 says
	// ``syntax error … `|' unexpected`` for `cat |& | wc -l`.
	if _, err := Parse(`cat |& | wc -l`, d); err == nil {
		t.Error("`cat |& | wc -l` parsed, want the second bar refused")
	}
	// And the pair the measurement turns on, which needs the other flag
	// from #1142 beside this one: a `;` may follow the operator and may not
	// follow a bar. Both halves are needed — the first alone would be
	// satisfied by a grammar that took a `;` anywhere, and
	// OneSeparatorExceptAfterABarOrBeforeACondition is the value that does not.
	lenient := d
	lenient.SeparatorWhereACommandBelongs = OneSeparatorExceptAfterABarOrBeforeACondition
	mustParse(t, `echo one |& ; echo two`, lenient, "a `;` may follow the operator")
	if _, err := Parse(`echo one | ; echo two`, lenient); err == nil {
		t.Error("`echo one | ; echo two` parsed, want a bar with no command refused")
	}
	// The same operator inside a compound body, which is a different parser
	// state and is where ksh93 was measured accepting it too.
	mustParse(t, `if true; then cat |&; fi`, lenient, "the operator inside a body")
}

// It terminates the whole **and-or**, the way `&` does rather than the way a
// pipe binds.
//
// Measured 2026-09-07: `echo A && cat |&` puts `echo A`'s output into the
// coprocess pipe, which a later `read -p` answers with `A` — so what went to
// the background is both commands and not only the one before the operator.
// The same for `echo A | cat |&`.
func TestTheCoprocessOperatorTakesTheWholeAndOr(t *testing.T) {
	d := withCoprocPipeOperator()
	for _, src := range []string{`echo A && cat |&`, `echo A | cat |&`} {
		f := mustFile(t, src, d)
		if n := len(f.Stmts); n != 1 {
			t.Fatalf("%s: statements = %d, want 1", src, n)
		}
		st := f.Stmts[0]
		if !st.Coprocess {
			t.Errorf("%s: statement = %+v, want a coprocess", src, st)
		}
		// The and-or is whole underneath it rather than having been cut at
		// the operator, which is the half a statement count cannot show.
		if _, isPipeline := st.Expr.(*Pipeline); isPipeline == (src == `echo A && cat |&`) {
			t.Errorf("%s: expression = %T, want the whole and-or", src, st.Expr)
		}
	}
}

// The two flags are the two readings of one spelling and never both apply.
//
// Without either the lexer makes no such token at all and the text is a bar
// then an ampersand, which is what bash 3.2 and dash lex — so the refusal
// lands where theirs does.
func TestTheTwoReadingsOfTheOperatorAreSeparate(t *testing.T) {
	if got, want := lex(t, `a |& b`, withCoprocPipeOperator()), `word(a) |& word(b)`; got != want {
		t.Errorf("with the coprocess reading: got %s, want %s", got, want)
	}
	if got, want := lex(t, `a |& b`, Core()), `word(a) | & word(b)`; got != want {
		t.Errorf("with neither: got %s, want %s", got, want)
	}
	// Under the pipe reading it joins two commands and the statement is not
	// a coprocess, which is the same text answering differently.
	f := mustFile(t, `a |& b`, withPipeBothStreams())
	if f.Stmts[0].Coprocess {
		t.Error("the pipe reading made a coprocess")
	}
	pl, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(pl.Cmds) != 2 {
		t.Errorf("expression = %T, want a pipeline of two", f.Stmts[0].Expr)
	}
}

// The printer writes the operator back, and the tree it re-reads to is the
// same one — which is the promise a background terminator has to keep like
// any other.
func TestTheCoprocessOperatorSurvivesPrinting(t *testing.T) {
	d := withCoprocPipeOperator()
	for _, src := range []string{`cat |&`, `echo A && cat |&`, `echo one |& echo two`} {
		f := mustFile(t, src, d)
		got := Print(f)
		again, err := Parse(got, d)
		if err != nil {
			t.Errorf("%s printed as %q, which does not parse: %v", src, got, err)
			continue
		}
		if Print(again) != got {
			t.Errorf("%s: printed %q, reprinted %q", src, got, Print(again))
		}
		if !again.Stmts[0].Coprocess {
			t.Errorf("%s: printed %q, which re-read as no coprocess", src, got)
		}
	}
}

// mustFile parses and fails the test rather than returning an error, for the
// rows above whose claim is about the tree rather than about acceptance.
func mustFile(t *testing.T, src string, d Dialect) *File {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return f
}
