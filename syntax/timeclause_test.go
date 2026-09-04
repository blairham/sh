// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// parseTimeClause parses src under d and returns the statement's TimeClause,
// failing the test when the tree has some other shape.
func parseTimeClause(t *testing.T, src string, d Dialect) *TimeClause {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("parse %q: %d statements, want 1", src, len(f.Stmts))
	}
	tc, ok := f.Stmts[0].Expr.(*TimeClause)
	if !ok {
		t.Fatalf("parse %q: %T, want *TimeClause", src, f.Stmts[0].Expr)
	}
	return tc
}

// TestTimeIsAKeywordWhereTheFlagSaysSo pins both sides of the flag: with it,
// `time` prefixes the whole pipeline; without it, `time` is an ordinary word
// and the same text is a simple command whose name is `time`.
func TestTimeIsAKeywordWhereTheFlagSaysSo(t *testing.T) {
	tc := parseTimeClause(t, `time true | wc -l`, Core())
	pl, ok := tc.Pipeline.(*Pipeline)
	if !ok {
		t.Fatalf("timed body is %T, want *Pipeline", tc.Pipeline)
	}
	if len(pl.Cmds) != 2 {
		t.Errorf("timed pipeline has %d elements, want 2 — `time` times the whole pipeline", len(pl.Cmds))
	}

	f, err := Parse(`time true`, POSIX())
	if err != nil {
		t.Fatalf("POSIX parse: %v", err)
	}
	pl, ok = f.Stmts[0].Expr.(*Pipeline)
	if !ok {
		t.Fatalf("POSIX tree is %T, want *Pipeline", f.Stmts[0].Expr)
	}
	sc, ok := pl.Cmds[0].(*SimpleCmd)
	if !ok || len(sc.Args) != 2 || sc.Args[0].Literal() != "time" {
		t.Errorf("without the flag `time` should be an ordinary command word, got %#v", pl.Cmds[0])
	}
}

// TestTimeIsOrdinaryOffTheFront covers where the keyword is *not* read: in
// the middle of a pipeline and after an assignment prefix, `time` stays a
// word — measured against bash, which runs the external in both places.
func TestTimeIsOrdinaryOffTheFront(t *testing.T) {
	for _, src := range []string{`echo hi | time wc -c`, `FOO=1 time true`} {
		f, err := Parse(src, Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if _, ok := f.Stmts[0].Expr.(*TimeClause); ok {
			t.Errorf("%q: parsed a TimeClause where `time` is an ordinary word", src)
		}
	}
}

// TestTimePosixFlagIsItsOwnQuestion: with the flag, `-p` belongs to `time`;
// without it, `-p` is the first word of the timed pipeline — which is what
// zsh does, a command that is not found, with the pipeline still timed.
func TestTimePosixFlagIsItsOwnQuestion(t *testing.T) {
	with := Core()
	with.TimePosixFlag = true
	tc := parseTimeClause(t, `time -p true`, with)
	if !tc.Posix {
		t.Error("`-p` not recorded where the dialect reads it")
	}

	tc = parseTimeClause(t, `time -p true`, Core())
	if tc.Posix {
		t.Error("`-p` read as a flag in a dialect without it")
	}
	pl, ok := tc.Pipeline.(*Pipeline)
	if !ok {
		t.Fatalf("timed body is %T, want *Pipeline", tc.Pipeline)
	}
	if sc, ok := pl.Cmds[0].(*SimpleCmd); !ok || sc.Args[0].Literal() != "-p" {
		t.Error("without the flag `-p` should be the pipeline's first word")
	}
}

// TestTimeSitsOnEitherSideOfTheBang: `time ! x` negates inside the clause and
// `! time x` outside it, and both parse — measured, both report and both
// carry status 1.
func TestTimeSitsOnEitherSideOfTheBang(t *testing.T) {
	tc := parseTimeClause(t, `time ! true`, Core())
	if tc.Negated {
		t.Error("`time ! x`: the bang is the pipeline's, not the clause's")
	}
	if pl, ok := tc.Pipeline.(*Pipeline); !ok || !pl.Negated {
		t.Error("`time ! x`: inner pipeline should be negated")
	}

	tc = parseTimeClause(t, `! time true`, Core())
	if !tc.Negated {
		t.Error("`! time x`: the clause should carry the bang")
	}
	if pl, ok := tc.Pipeline.(*Pipeline); !ok || pl.Negated {
		t.Error("`! time x`: inner pipeline should not be negated")
	}
}

// TestBareTimeParsesAndAPipeAfterItDoesNot: `time` alone reports on nothing,
// and `time | cat` is a pipe with no first element — bash calls it a syntax
// error and so do we.
func TestBareTimeParsesAndAPipeAfterItDoesNot(t *testing.T) {
	tc := parseTimeClause(t, `time`, Core())
	if tc.Pipeline != nil {
		t.Errorf("bare `time` should have no pipeline, got %T", tc.Pipeline)
	}
	// A bare `time` before `&&` still parses: the `&&` is the caller's.
	f, err := Parse(`time && echo ok`, Core())
	if err != nil {
		t.Fatalf("`time && echo ok`: %v", err)
	}
	if _, ok := f.Stmts[0].Expr.(*BinaryExpr); !ok {
		t.Errorf("`time && echo ok` is %T, want *BinaryExpr", f.Stmts[0].Expr)
	}

	if _, err := Parse(`time | cat`, Core()); err == nil {
		t.Error("`time | cat` parsed, want a syntax error")
	}
}

// TestTimePrintingRoundTrips: printed text has to parse and print the same
// again — the corpus round-trip promise, made here for the forms the corpus
// does not carry.
func TestTimePrintingRoundTrips(t *testing.T) {
	d := Core()
	d.TimePosixFlag = true
	for _, src := range []string{
		`time true`,
		`time true 2>&1 | wc -l`,
		`time -p true`,
		`time ! true`,
		`! time true`,
		`time`,
		`time true && echo ok`,
		`{ time true; } 2>&1 | grep -c real`,
	} {
		first, err := Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		printed := Print(first)
		second, err := Parse(printed, d)
		if err != nil {
			t.Fatalf("printed %q from %q does not parse: %v", printed, src, err)
		}
		if again := Print(second); again != printed {
			t.Errorf("printing is not settled:\n  from:  %s\n  once:  %s\n  twice: %s", src, printed, again)
		}
	}
}
