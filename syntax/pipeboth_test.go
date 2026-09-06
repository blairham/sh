// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// withPipeBothStreams is Core plus the one flag under test. A core test names
// a flag rather than a shell: which shells set it is the dialect packages'
// business, and this file must not know they exist.
func withPipeBothStreams() Dialect {
	d := Core()
	d.PipeBothStreams = true
	return d
}

// `|&` is one token where the dialect has it and two where it does not, and
// the two are the reading the shells without the operator give the same text:
// a bar, then an ampersand. Nothing is invented in either direction.
func TestPipeBothStreamsIsADialectDecision(t *testing.T) {
	if got, want := lex(t, `a |& b`, withPipeBothStreams()), `word(a) |& word(b)`; got != want {
		t.Errorf("with the operator: got %s, want %s", got, want)
	}
	if got, want := lex(t, `a |& b`, Core()), `word(a) | & word(b)`; got != want {
		t.Errorf("without it: got %s, want %s", got, want)
	}
}

// The two bytes have to be adjacent. `a | & b` is not the operator in any
// shell that has it, and reading it as one would take a construct away from
// the dialects that spell a background command that way.
func TestPipeBothStreamsNeedsNoBlankBetween(t *testing.T) {
	if got, want := lex(t, `a | & b`, withPipeBothStreams()), `word(a) | & word(b)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := Parse("a | & b", withPipeBothStreams()); err == nil {
		t.Error("`a | & b` parsed, want a refusal")
	}
}

// The flag gates the construct rather than the text: without it the line does
// not silently mean something else, it is refused where the shells refuse it.
func TestPipeBothStreamsGatesTheConstruct(t *testing.T) {
	mustParse(t, `a |& b`, withPipeBothStreams(), "with the operator")
	mustParse(t, `{ a; } |& b |& c`, withPipeBothStreams(), "chained and compound")
	mustParse(t, `for i in 1; do a |& b; done`, withPipeBothStreams(), "inside a loop")
	mustFail(t, `a |& b`, Core(), "without the operator")
	mustFail(t, `a |& b`, POSIX(), "without the operator")
}

// What `|&` means: a `2>&1` on the command before it, and the *last* one that
// command has. The order is the whole of the operator — see the flag's own
// comment for the two shapes that measure it — so the position in the list is
// asserted rather than merely its presence.
func TestPipeBothStreamsIsATrailingRedirection(t *testing.T) {
	f, err := Parse(`a 2>/dev/null |& b`, withPipeBothStreams())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pl, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok {
		t.Fatalf("want a pipeline, got %T", f.Stmts[0].Expr)
	}
	left, ok := pl.Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("want a simple command, got %T", pl.Cmds[0])
	}
	if n := len(left.Redirs); n != 2 {
		t.Fatalf("want the written redirection and the implied one, got %d", n)
	}
	if left.Redirs[0].PipeBoth {
		t.Error("the written redirection is marked as implied")
	}
	rd := left.Redirs[1]
	if !rd.PipeBoth {
		t.Error("the implied redirection is not marked, so the printer would write it out")
	}
	if rd.Op != TokGreatAmp {
		t.Errorf("op %s, want %s", rd.Op, TokGreatAmp)
	}
	if got := rd.N.Literal(); got != "2" {
		t.Errorf("descriptor %q, want %q", got, "2")
	}
	if got := rd.Word.Literal(); got != "1" {
		t.Errorf("target %q, want %q", got, "1")
	}
	if rd.Text != "1" {
		t.Errorf("text %q, want %q", rd.Text, "1")
	}
	// The bar it stands for, so a diagnostic about it points at the operator
	// the script wrote and not at a byte nobody typed.
	if want := (Pos{Offset: 14, Line: 1, Col: 15}); rd.OpPos != want {
		t.Errorf("position %+v, want %+v", rd.OpPos, want)
	}
}

// Only the bar the operator was written on carries it. A pipeline that mixes
// the two spellings has to keep them apart, which a single flag on the
// pipeline could not do.
func TestPipeBothStreamsIsPerBar(t *testing.T) {
	f, err := Parse(`a |& b | c`, withPipeBothStreams())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pl := f.Stmts[0].Expr.(*Pipeline)
	for i, want := range []bool{true, false, false} {
		if got := mergesStderr(pl.Cmds[i]); got != want {
			t.Errorf("element %d merges=%v, want %v", i, got, want)
		}
	}
}

// The printer writes the operator back rather than the redirection it means.
// A round trip that spelled it `2>&1 |` would still parse and still be
// settled, and would have thrown away what the script said.
func TestPrintingPipeBothStreams(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a |& b`, `a |& b`},
		{`a 2>/dev/null |& b`, `a 2> /dev/null |& b`},
		{`a |& b |& c`, `a |& b |& c`},
		{`a |& b | c`, `a |& b | c`},
		{`a | b |& c`, `a | b |& c`},
		{`! a |& b`, `! a |& b`},
		{`{ a; } |& b`, `{ a; } |& b`},
		{`a 2>&1 | b`, `a 2>&1 | b`},
		// A function definition has no redirection list of its own, so the
		// operator is carried by its *body* — which is where the parser puts
		// a written redirection there too. The spelling has to come back off
		// the body as well, or this line prints as a plain bar.
		{`f() { :; } |& b`, `f() { :; } |& b`},
		{`f() ( : ) |& b`, `f() ( : ) |& b`},
	} {
		f, err := Parse(tc.src, withPipeBothStreams())
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		got := Print(f)
		if got != tc.want {
			t.Errorf("%q printed as %q, want %q", tc.src, got, tc.want)
			continue
		}
		again, err := Parse(got, withPipeBothStreams())
		if err != nil {
			t.Errorf("%q: printed source does not parse: %v", tc.src, err)
			continue
		}
		if settled := Print(again); settled != got {
			t.Errorf("%q: printing is not settled: %q then %q", tc.src, got, settled)
		}
	}
}

// A bar with nothing after it names the token it found, not the bar. That is
// what every shell in the panel that refuses these does, and it is the half
// of #1115 that was wrong in every dialect: the substrate invented a sentence
// — "expected a command after |" — that no shell writes.
func TestABarWithNoCommandNamesTheToken(t *testing.T) {
	for _, tc := range []struct {
		src, token string
		class      TokenClass
		d          Dialect
	}{
		{"a | ; b", ";", ClassOperator, Core()},
		{"a | & b", "&", ClassOperator, Core()},
		{"a |& b", "&", ClassOperator, Core()},
		{"a | | b", "|", ClassOperator, Core()},
		{"a | && b", "&&", ClassOperator, Core()},
		{"a |& ; b", ";", ClassOperator, withPipeBothStreams()},
		// A command may begin after a bar, so a reserved word standing
		// there is reserved. The class travels because one dialect names it
		// rather than the token, and it names an *ordinary* word — so
		// getting this wrong turns `"fi" unexpected` into `word unexpected`
		// there and nowhere else.
		{"a | fi", "fi", ClassReserved, Core()},
		{"a | done", "done", ClassReserved, Core()},
		{"a | esac", "esac", ClassReserved, Core()},
	} {
		_, err := Parse(tc.src, tc.d)
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", tc.src, err)
			continue
		}
		if se.Kind != ErrUnexpected {
			t.Errorf("%q: kind %v, want ErrUnexpected", tc.src, se.Kind)
		}
		if se.Token != tc.token {
			t.Errorf("%q: blamed %q, want %q", tc.src, se.Token, tc.token)
		}
		if se.Class != tc.class {
			t.Errorf("%q: class %v, want %v", tc.src, se.Class, tc.class)
		}
	}
}

// Input that ran out after a bar is unfinished rather than wrong, and stays
// so: it is the one case the token-naming path must not take, because there
// is no token to name.
func TestABarAtTheEndOfInputIsUnfinished(t *testing.T) {
	for _, src := range []string{"a |", "a |&"} {
		_, err := Parse(src, withPipeBothStreams())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want a *syntax.Error", src, err)
			continue
		}
		if se.Kind != ErrUnterminated {
			t.Errorf("%q: kind %v, want ErrUnterminated", src, se.Kind)
		}
	}
}

// A `|&` waiting for its command is open under its own spelling, not under
// the bar's. That is the one surface on which the two are distinguishable to
// the person typing: zsh's continuation prompt names them apart, `pipe`
// against `errpipe`, so the word the parser reports has to be the one that
// was written.
//
//	% PS2='[%_]'
//	% echo a |
//	[pipe]cat
//	% echo b |&
//	[errpipe]cat
func TestAPipeOfBothStreamsIsOpenUnderItsOwnName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo x |\n", "|"},
		{"echo x |&\n", "|&"},
	} {
		p := NewParser(tc.src, withPipeBothStreams())
		p.Parse()
		open := p.Open()
		if len(open) != 1 {
			t.Errorf("%q: open = %v, want one thing", tc.src, open)
			continue
		}
		if open[0].Word != tc.want {
			t.Errorf("%q: open as %q, want %q", tc.src, open[0].Word, tc.want)
		}
	}
}
