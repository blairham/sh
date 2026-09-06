// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"strings"
	"testing"
)

// dump renders a tree as an s-expression so a test can state its expectation
// as one readable string. Structure is what these tests are about, so the
// shape is deliberately explicit about nesting.
func dump(n Node) string {
	switch x := n.(type) {
	case *File:
		return join(stmts(x.Stmts))
	case *Stmt:
		s := dump(x.Expr)
		if x.Background {
			s = "bg(" + s + ")"
		}
		return s
	case *BinaryExpr:
		return "(" + dump(x.X) + " " + x.Op.String() + " " + dump(x.Y) + ")"
	case *Pipeline:
		parts := make([]string, len(x.Cmds))
		for i, c := range x.Cmds {
			parts[i] = dump(c)
		}
		s := strings.Join(parts, " | ")
		if len(x.Cmds) > 1 {
			s = "pipe(" + s + ")"
		}
		if x.Negated {
			s = "!" + s
		}
		return s
	case *SimpleCmd:
		var b []string
		for _, a := range x.Assigns {
			b = append(b, a.Name+"="+a.Value.Literal())
		}
		for _, w := range x.Args {
			b = append(b, w.Literal())
		}
		s := "cmd[" + strings.Join(b, " ") + "]"
		return s + redirStr(x.Redirs)
	case *Subshell:
		return "sub{" + join(stmts(x.List)) + "}" + redirStr(x.Redirs)
	case *Group:
		return "grp{" + join(stmts(x.List)) + "}" + redirStr(x.Redirs)
	case *IfClause:
		s := "if[" + join(stmts(x.Cond)) + "]then[" + join(stmts(x.Then)) + "]"
		for _, e := range x.Elifs {
			s += "elif[" + join(stmts(e.Cond)) + "]then[" + join(stmts(e.Then)) + "]"
		}
		if x.HasElse {
			s += "else[" + join(stmts(x.Else)) + "]"
		}
		return s + redirStr(x.Redirs)
	case *LoopClause:
		kw := "while"
		if x.Until {
			kw = "until"
		}
		return kw + "[" + join(stmts(x.Cond)) + "]do[" + join(stmts(x.Body)) + "]" + redirStr(x.Redirs)
	case *ForClause:
		items := "no-list"
		if x.HasItems {
			var ws []string
			for _, w := range x.Items {
				ws = append(ws, w.Literal())
			}
			items = "in(" + strings.Join(ws, ",") + ")"
		}
		return "for " + strings.Join(x.Names, " ") + " " + items + " do[" + join(stmts(x.Body)) + "]" + redirStr(x.Redirs)
	case *SelectClause:
		items := "no-list"
		if x.HasItems {
			var ws []string
			for _, w := range x.Items {
				ws = append(ws, w.Literal())
			}
			items = "in(" + strings.Join(ws, ",") + ")"
		}
		return "select " + x.Name + " " + items + " do[" + join(stmts(x.Body)) + "]" + redirStr(x.Redirs)
	case *CaseClause:
		s := "case " + x.Word.Literal() + "{"
		for _, it := range x.Items {
			var pats []string
			for _, p := range it.Patterns {
				pats = append(pats, p.Literal())
			}
			s += strings.Join(pats, "|") + ")" + join(stmts(it.Body)) + it.Term.String() + " "
		}
		return strings.TrimSpace(s) + "}" + redirStr(x.Redirs)
	case *ArithCmdClause:
		return "arith{" + strings.TrimSpace(x.Expr) + "}" + redirStr(x.Redirs)
	case *FuncDecl:
		kw := ""
		if x.Keyword {
			kw = "kw:"
		}
		return "func " + kw + x.Name + " " + dump(x.Body)
	}
	return fmt.Sprintf("?%T", n)
}

func stmts(list []*Stmt) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = dump(s)
	}
	return out
}

func join(parts []string) string { return strings.Join(parts, "; ") }

func redirStr(rs []*Redirect) string {
	if len(rs) == 0 {
		return ""
	}
	var b []string
	for _, r := range rs {
		s := ""
		if r.N != nil {
			s += r.N.Literal()
		}
		s += r.Op.String() + r.Word.Literal()
		b = append(b, s)
	}
	return "<" + strings.Join(b, ",") + ">"
}

func parse(t *testing.T, src string, d Dialect) string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return dump(f)
}

func TestAndOrIsOneLeftAssociativeLevel(t *testing.T) {
	// The discriminating case. C's precedence would give
	// (true || (echo A && echo B)) and print nothing; every shell prints B.
	if got, want := parse(t, `true || echo A && echo B`, Core()),
		`((cmd[true] || cmd[echo A]) && cmd[echo B])`; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	if got, want := parse(t, `a && b && c`, Core()),
		`((cmd[a] && cmd[b]) && cmd[c])`; got != want {
		t.Errorf("chain: got %s\nwant %s", got, want)
	}
}

func TestPipelineAndNegation(t *testing.T) {
	// `!` applies to the whole pipeline, not to its first command.
	if got, want := parse(t, `! true | false`, Core()), `!pipe(cmd[true] | cmd[false])`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got, want := parse(t, `a | b | c`, Core()), `pipe(cmd[a] | cmd[b] | cmd[c])`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestRedirectionsAreNotPositional(t *testing.T) {
	// They may precede the command name or sit between arguments, so they are
	// lifted out of the word list rather than expected as a suffix.
	tests := []struct{ src, want string }{
		{`>b echo hi`, `cmd[echo hi]<>b>`},
		{`echo one >b two`, `cmd[echo one two]<>b>`},
		{`echo 1>b`, `cmd[echo]<1>b>`},
		{`echo 1 >b`, `cmd[echo 1]<>b>`},
		{`a <in >out`, `cmd[a]<<in,>out>`},
	}
	for _, tc := range tests {
		if got := parse(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestAssignmentsAreSeparatedFromArguments(t *testing.T) {
	tests := []struct{ src, want string }{
		{`x=1`, `cmd[x=1]`},
		{`x=1 y=2 cmd a`, `cmd[x=1 y=2 cmd a]`},
		// After a command name a word that looks like an assignment is an
		// argument, which is why the parser tracks whether one has been seen.
		{`echo x=1`, `cmd[echo x=1]`},
	}
	for _, tc := range tests {
		if got := parse(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
	f, _ := Parse(`x=1 echo hi`, Core())
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(sc.Assigns) != 1 || sc.Assigns[0].Name != "x" {
		t.Errorf("assignment not separated: %+v", sc.Assigns)
	}
	if len(sc.Args) != 2 {
		t.Errorf("want 2 args, got %d", len(sc.Args))
	}
}

func TestGroupingAndCompoundRedirections(t *testing.T) {
	tests := []struct{ src, want string }{
		{`(a; b)`, `sub{cmd[a]; cmd[b]}`},
		{`{ a; b; }`, `grp{cmd[a]; cmd[b]}`},
		// A redirection on a compound command applies to everything inside,
		// so every compound node carries its own list.
		{`{ a; } >f`, `grp{cmd[a]}<>f>`},
		{`(a) >f`, `sub{cmd[a]}<>f>`},
		{`for i in 1; do a; done >f`, `for i in(1) do[cmd[a]]<>f>`},
	}
	for _, tc := range tests {
		if got := parse(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestIfChain(t *testing.T) {
	src := `if false; then a; elif true; then b; else c; fi`
	want := `if[cmd[false]]then[cmd[a]]elif[cmd[true]]then[cmd[b]]else[cmd[c]]`
	if got := parse(t, src, Core()); got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	// The condition is a list judged by its last command, so it holds more
	// than one statement.
	if got, want := parse(t, `if false; true; then a; fi`, Core()),
		`if[cmd[false]; cmd[true]]then[cmd[a]]`; got != want {
		t.Errorf("list condition: got %s\nwant %s", got, want)
	}
}

func TestLoops(t *testing.T) {
	if got, want := parse(t, `while a; do b; done`, Core()), `while[cmd[a]]do[cmd[b]]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got, want := parse(t, `until a; do b; done`, Core()), `until[cmd[a]]do[cmd[b]]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestForDistinguishesAbsentFromEmptyList(t *testing.T) {
	// The measured difference: with `in` omitted the loop iterates the
	// positional parameters; with `in` and nothing after it, nothing. A nil
	// slice cannot say which, so the AST carries HasItems.
	if got, want := parse(t, `for i; do a; done`, Core()), `for i no-list do[cmd[a]]`; got != want {
		t.Errorf("absent: got %s, want %s", got, want)
	}
	if got, want := parse(t, `for i in; do a; done`, Core()), `for i in() do[cmd[a]]`; got != want {
		t.Errorf("empty: got %s, want %s", got, want)
	}
	if got, want := parse(t, `for i in a b; do c; done`, Core()), `for i in(a,b) do[cmd[c]]`; got != want {
		t.Errorf("items: got %s, want %s", got, want)
	}
	// A newline separates wherever ; does.
	if got, want := parse(t, "for i in a\ndo c; done", Core()), `for i in(a) do[cmd[c]]`; got != want {
		t.Errorf("newline: got %s, want %s", got, want)
	}
}

func TestCase(t *testing.T) {
	tests := []struct{ src, want string }{
		{`case x in a) b;; esac`, `case x{a)cmd[b];;}`},
		{`case x in (a) b;; esac`, `case x{a)cmd[b];;}`},
		{`case x in a|b) c;; esac`, `case x{a|b)cmd[c];;}`},
		{`case x in a) ;; esac`, `case x{a);;}`},
		{`case x in a) b;& c) d;; esac`, `case x{a)cmd[b];& c)cmd[d];;}`},
	}
	for _, tc := range tests {
		if got := parse(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestFunctionForms(t *testing.T) {
	if got, want := parse(t, `f() { a; }`, Core()), `func f grp{cmd[a]}`; got != want {
		t.Errorf("posix: got %s, want %s", got, want)
	}
	if got, want := parse(t, `function f { a; }`, Core()), `func kw:f grp{cmd[a]}`; got != want {
		t.Errorf("keyword: got %s, want %s", got, want)
	}
	// A body is any compound command, not only a brace group.
	if got, want := parse(t, `f() ( a )`, Core()), `func f sub{cmd[a]}`; got != want {
		t.Errorf("subshell body: got %s, want %s", got, want)
	}
}

func TestArithmeticCommandParses(t *testing.T) {
	if got, want := parse(t, `(( 1+1 ))`, Core()), `arith{1+1}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestBackgroundBelongsToTheStatement(t *testing.T) {
	// `a && b &` backgrounds the whole and-or, not just b.
	if got, want := parse(t, `a && b &`, Core()), `bg((cmd[a] && cmd[b]))`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestReservedWordsAreOnlyReservedInCommandPosition(t *testing.T) {
	// `echo if then done` passes them through as arguments.
	if got, want := parse(t, `echo if then done`, Core()), `cmd[echo if then done]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// Quoting removes the reservation entirely.
	if got, want := parse(t, `"if" x`, Core()), `cmd[if x]`; got != want {
		t.Errorf("quoted: got %s, want %s", got, want)
	}
}

func TestParserIncompleteIsNotInvalid(t *testing.T) {
	// A half-typed line is the normal case at a prompt.
	for _, src := range []string{
		`if true; then`, `while a; do`, `for i in a; do`, `case x in`,
		`(a`, `{ a;`, `a &&`, `f() {`,
		// Unfinished rather than wrong, and the shells agree: dash reports
		// `end of file unexpected (expecting "}")` and bash `unexpected end
		// of file`. The brace was consumed as an argument, because a brace
		// group needs a terminator before it.
		`{ echo a }`,
	} {
		p := NewParser(src, Core())
		p.Parse()
		if p.Err() == nil {
			t.Errorf("%q: want an error", src)
			continue
		}
		if !p.Incomplete() {
			t.Errorf("%q: want Incomplete, got %v", src, p.Err())
		}
	}
}

func TestInvalidIsNotIncomplete(t *testing.T) {
	// Something genuinely wrong, with input still to come, is an error to
	// report now rather than a prompt for more.
	for _, src := range []string{
		`case x in a b;; esac`, // a pattern must be followed by )
		`for 1 in a; do b; done`,
	} {
		p := NewParser(src, Core())
		p.Parse()
		if p.Err() == nil {
			t.Errorf("%q: want an error", src)
			continue
		}
		if p.Incomplete() {
			t.Errorf("%q: reported incomplete, but it is wrong rather than unfinished", src)
		}
	}
}

func TestParserNeverPanics(t *testing.T) {
	inputs := []string{
		"", " ", "\n", ";", "&", "|", "||", "&&", "(", ")", "()", "{", "}",
		"if", "then", "fi", "do", "done", "esac", "case", "for", "while",
		"if;then;fi", "case in esac", "for in do done", "a|", "|a", "!",
		"f()", "f()(", "((", "(( ", "[[", "]]", ";;", ";&", "a;;b",
		strings.Repeat("(", 200), strings.Repeat("if ", 100),
	}
	for _, src := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			for _, d := range []Dialect{Core(), POSIX(), everyFlag()} {
				NewParser(src, d).Parse()
			}
		}()
	}
}

func FuzzParserNeverPanics(f *testing.F) {
	for _, s := range []string{
		`a && b || c`, `if x; then y; fi`, `for i in a; do b; done`,
		`case x in a) b;; esac`, `f() { a; }`, `{ a; } >f`, `(( 1+1 ))`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		file := NewParser(src, Core()).Parse()
		if file == nil {
			t.Fatal("Parse returned nil")
		}
		// Whatever the input, dumping the tree must also not panic: it is the
		// closest thing to a consumer walking the nodes.
		_ = dump(file)
	})
}

func heredocs(t *testing.T, src string) []*Redirect {
	t.Helper()
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("want a simple command in %q", src)
	}
	return sc.Redirs
}

func TestHeredocBodyStartsAfterTheNextNewline(t *testing.T) {
	// Not after the operator: the rest of the line is ordinary input and is
	// read first. This is why collecting the body needs the lexer and the
	// parser to cooperate — the parser has the delimiter, the lexer reaches
	// the newline.
	rs := heredocs(t, "cat <<EOF; echo after\none\nEOF\n")
	if len(rs) != 1 {
		t.Fatalf("want 1 redirect, got %d", len(rs))
	}
	if got := rs[0].Heredoc.Literal(); got != "one\n" {
		t.Errorf("body = %q, want %q", got, "one\n")
	}
}

func TestHeredocEndsOnAnExactDelimiterLine(t *testing.T) {
	rs := heredocs(t, "cat <<EOF\nEOFX\nEOF\n")
	if got := rs[0].Heredoc.Literal(); got != "EOFX\n" {
		t.Errorf("body = %q: EOFX must not end an EOF heredoc", got)
	}
}

func TestHeredocDashStripsTabsOnly(t *testing.T) {
	rs := heredocs(t, "cat <<-EOF\n\ttabbed\n\tEOF\n")
	if got := rs[0].Heredoc.Literal(); got != "tabbed\n" {
		t.Errorf("body = %q, want tabs stripped", got)
	}
	// Spaces are not stripped, so a space-indented delimiter never matches and
	// the heredoc runs to the end of input — unfinished, not wrong.
	p := NewParser("cat <<-EOF\n    spaced\n    EOF\n", Core())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("unfinished is not wrong: %v", err)
	}
	if !p.Incomplete() {
		t.Error("want Incomplete, so a prompt asks for another line")
	}
	body := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Redirs[0].Heredoc.Literal()
	if body != "    spaced\n    EOF\n" {
		t.Errorf("body = %q, want everything to the end of input", body)
	}
}

func TestSeveralHeredocsAreCollectedInOperatorOrder(t *testing.T) {
	rs := heredocs(t, "cat <<A <<B\nfirst\nA\nsecond\nB\n")
	if len(rs) != 2 {
		t.Fatalf("want 2 redirects, got %d", len(rs))
	}
	if got := rs[0].Heredoc.Literal(); got != "first\n" {
		t.Errorf("first body = %q", got)
	}
	if got := rs[1].Heredoc.Literal(); got != "second\n" {
		t.Errorf("second body = %q", got)
	}
}

func TestHeredocDelimiterQuotingReachesTheBody(t *testing.T) {
	// Any quoting anywhere in the delimiter makes the whole body literal. The
	// body is kept raw either way; the flag is what tells expansion whether to
	// re-read it.
	for _, src := range []string{
		"cat <<\"EOF\"\n$x\nEOF\n",
		"cat <<'EOF'\n$x\nEOF\n",
		"cat <<\\EOF\n$x\nEOF\n",
	} {
		rs := heredocs(t, src)
		if q := rs[0].Heredoc.Spans[0].Quoting; q == Unquoted {
			t.Errorf("%q: body marked unquoted, but the delimiter was quoted", src)
		}
	}
	rs := heredocs(t, "cat <<EOF\n$x\nEOF\n")
	if q := rs[0].Heredoc.Spans[0].Quoting; q != Unquoted {
		t.Errorf("unquoted delimiter should leave the body expandable, got %v", q)
	}
}

func TestHeredocUnterminatedIsIncomplete(t *testing.T) {
	p := NewParser("cat <<EOF\nbody\n", Core())
	p.Parse()
	if !p.Incomplete() {
		t.Error("want Incomplete when the delimiter never arrives")
	}
}

// TestNextLineReadsOneLogicalLine: the unit a shell reads before it runs
// anything. It is a *line* and not a statement, because `echo one; { fi; }`
// runs neither half — and it stretches past a newline while a construct is
// still open, or the loop could never be read at all.
func TestNextLineReadsOneLogicalLine(t *testing.T) {
	for _, tc := range []struct {
		src   string
		lines []int // statements in each logical line
	}{
		{"echo one\necho two\n", []int{1, 1}},
		{"echo one; echo two\necho three\n", []int{2, 1}},
		{"echo one\n\n\necho two\n", []int{1, 1}},
		// A construct holds the line open across its newlines.
		{"for i in 1 2\ndo\n echo $i\ndone\necho after\n", []int{1, 1}},
		{"if true\nthen\n echo x\nfi\n", []int{1}},
		// A trailing `&` ends a statement without ending the line.
		{"echo one & echo two\n", []int{2}},
		{"", nil},
		{"\n\n", nil},
	} {
		p := NewParser(tc.src, Core())
		var got []int
		for {
			line, ok := p.NextLine()
			if !ok {
				break
			}
			got = append(got, len(line.Stmts))
		}
		if err := p.Err(); err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		if len(got) != len(tc.lines) {
			t.Errorf("%q: %d lines, want %d", tc.src, len(got), len(tc.lines))
			continue
		}
		for i := range got {
			if got[i] != tc.lines[i] {
				t.Errorf("%q: line %d has %d statements, want %d", tc.src, i+1, got[i], tc.lines[i])
			}
		}
	}
}

// TestParseIsEveryLine: reading the whole input is the same as reading it a
// line at a time, so nothing depends on which a caller chose.
func TestParseIsEveryLine(t *testing.T) {
	const src = "echo one; echo two\nfor i in 1 2\ndo\n echo $i\ndone\nif true; then :; fi\n"
	whole := parse(t, src, Core())

	p := NewParser(src, Core())
	var f File
	for {
		line, ok := p.NextLine()
		if !ok {
			break
		}
		f.Stmts = append(f.Stmts, line.Stmts...)
	}
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	if got := dump(&f); got != whole {
		t.Errorf("line at a time gave\n%s\nwhole gave\n%s", got, whole)
	}
}
