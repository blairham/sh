// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The corpus round trip lives in printroundtrip_test.go, where it compares
// the trees rather than only reading the printed source back — the version
// that stood here could not tell a pattern group from three escaped
// characters (#1221).

// What the corpus could not reach.
//
// Found by printing every shell script installed on this machine and reading
// it back — 163 of them parse, and nine did not survive the round trip. The
// corpus has 490 snippets and none of them does either of these, because
// nobody writes a case to exercise a printer.
func TestPrintingWhatTheCorpusDoesNotReach(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			// A here-document body ends the line, so what closes the block
			// is at the start of the next one and `; }` there is a `;` with
			// nothing before it. Four scripts on this machine close a block
			// straight after a here-document.
			"a block closed after a here-document",
			"f() { cat <<EOF\nbody\nEOF\n}\n",
		},
		{
			"a conditional closed after one",
			"if true; then cat <<EOF\nbody\nEOF\nfi\n",
		},
		{
			"an arm closed after one",
			"case x in a) cat <<EOF\nbody\nEOF\n;; esac\n",
		},
		{
			// Backticks and `$( )` are one node, so every substitution is
			// written the second way — and a backtick one has no space to
			// inherit. `$((` is arithmetic, so a substitution whose first
			// command is a subshell needs one put in: the same trap as a
			// redirection target beginning with `<`, and the reason
			// /opt/homebrew/bin/gettext.sh could not be reprinted.
			"a backtick substitution beginning with a subshell",
			"x=`(echo hi) 2>/dev/null`\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := syntax.Print(f)
			if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
				t.Fatalf("printed source does not parse: %v\n  from: %q\n  gave: %q", err, tc.src, printed)
			}
		})
	}
}

// A dup redirection, which is spelled unlike every other one.
//
// Two rules, and the exact spelling is the whole of what is being tested, so
// these assert the text rather than that it parses. The operator goes tight
// against its target where every other redirection takes a space; and where
// the target is one the reader can resolve — a bare number, or the `-` that
// closes a descriptor — the descriptor being redirected is written out, which
// nothing in the source had to say.
//
// The oracle is `type`: the only place any shell in the panel says a function
// body back, and so the only place a printer can be checked against something
// other than its own opinion.
func TestPrintingADupRedirection(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the descriptor read from is filled in", "echo hi >&2", "echo hi 1>&2"},
		{"and the one read into", "echo hi <&3", "echo hi 0<&3"},
		{"one already written stays as it is", "echo hi 2>&1", "echo hi 2>&1"},
		{"and is not rewritten to the operator's own", "echo hi 3<&0", "echo hi 3<&0"},
		{"a close names its descriptor too", "echo hi >&-", "echo hi 1>&-"},
		{
			// The operator changes and the descriptor does not: `<&-` closes
			// standard input, and is written as a close of it rather than as
			// a close of standard output.
			"a close is written one way whichever asked for it",
			"echo hi <&-", "echo hi 0>&-",
		},
		{"a close that named one keeps it", "echo hi 2>&-", "echo hi 2>&-"},
		{
			// Nothing here can say which descriptor this is: the word is
			// settled when the command runs. Writing one in would be
			// inventing it.
			"a target that is not settled yet gets nothing filled in",
			"echo hi >&$fd", "echo hi >&$fd",
		},
		{
			// Quoting is what tells a descriptor from a file of that name,
			// so a quoted number is not a descriptor to fill in from.
			"a quoted number is not a descriptor",
			"echo hi >&\"1\"", "echo hi >&\"1\"",
		},
		{"nor is a word", "echo hi >&x", "echo hi >&x"},
		{"but the tightness is not conditional on any of that", "echo hi >&/dev/null", "echo hi >&/dev/null"},
		{
			// The operator that redirects both streams ends in the same
			// character and is not a dup: it takes a file, and so takes the
			// space and has no descriptor to fill in.
			"the both-streams operator is not one of these",
			"echo hi &>out", "echo hi &> out",
		},
		{"nor is its appending form", "echo hi &>>out", "echo hi &>> out"},
		{"where a file target keeps the space", "echo hi >out", "echo hi > out"},
		{"and keeps only the descriptor that was written", "echo hi 2>out", "echo hi 2> out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := strings.TrimRight(syntax.Print(f), "\n"); got != tc.want {
				t.Errorf("printing %q gave %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A translatable string prints as a plain double-quoted one.
//
// The `$` of `$"..."` contributes nothing to the tree — the spans are exactly
// what a bare `"` produces — so the printed form drops it rather than record a
// byte the tree holds no question for. The assertion is the round trip: the
// printed source must parse to the same tree in the dialect that read the
// original, and it does trivially, because it *is* the plain-quote spelling of
// that tree. It also now parses identically in a dialect without the flag,
// which the original did not.
func TestPrintingATranslatableString(t *testing.T) {
	d := syntax.Core()
	d.DollarDoubleQuote = true
	for _, tc := range []struct{ name, src, want string }{
		{"the dollar is dropped", `echo $"a b"`, `echo "a b"`},
		{"expansions inside survive", `echo $"hi $x"`, `echo "hi $x"`},
		{"mid-word, like any quoting", `echo a$"b c"d`, `echo a"b c"d`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := strings.TrimRight(syntax.Print(f), "\n")
			if printed != tc.want {
				t.Errorf("printing %q gave %q, want %q", tc.src, printed, tc.want)
			}
			// The round trip, in both dialects: the printed form must mean
			// the same thing wherever it lands.
			for _, rd := range []syntax.Dialect{d, syntax.Core()} {
				second, err := syntax.Parse(printed, rd)
				if err != nil {
					t.Fatalf("printed form %q does not reparse: %v", printed, err)
				}
				if again := strings.TrimRight(syntax.Print(second), "\n"); again != printed {
					t.Errorf("printing is not settled: once %q, twice %q", printed, again)
				}
			}
		})
	}
}

// TestABackgroundStatementBeforeAClosingWord — the printer wrote the `&` and
// then the `;` separator as well, so `f() { true & }` printed as
// `f() { true &; }`, which parses nowhere. The one file of 1093 installed
// scripts that failed the parse→print→reparse sweep reduced to this.
func TestABackgroundStatementBeforeAClosingWord(t *testing.T) {
	for _, src := range []string{
		"f() { true & }",
		"{ true & }",
		"f() { if true; then true & fi; }",
		"while true; do true & done",
		"( true & )",
		"f() { true & true; }",
		"{ true & true & }",
	} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		printed := syntax.Print(f)
		if strings.Contains(printed, "&;") {
			t.Errorf("%q printed as %q: `&;` parses nowhere", src, printed)
		}
		if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
			t.Errorf("%q printed as %q, which does not reparse: %v", src, printed, err)
		}
	}
}

// The `function` keyword is written back where it was written.
//
// The whole rendered text is asserted rather than that it contains the word,
// because the failure this replaces produced output that would satisfy a
// `contains`: `f() { …; }` with the keyword bolted on the front is neither
// spelling, and only comparing the line can tell "the word is there" from
// "the declaration is right".
//
// The oracle is ksh93, the one shell in the panel for which the word means
// anything and the one that says it back: `function f { echo hi; }` listed
// with `typeset -f` there is `function f { echo hi; };` and `f() { echo hi;
// }` is `f() { echo hi; };`. Measured 2026-09-08 on ksh93u+ 2012-08-01. bash
// 5.3, bash 3.2 and zsh 5.9.2 all normalise both to `f () `, which they may:
// `typeset` declares a local in either body there, so nothing distinguishes
// them to be lost. dash has no `function` keyword at all and no tree here
// can carry the flag under its grammar.
func TestTheFunctionKeywordIsPrintedBack(t *testing.T) {
	d := syntax.Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.FunctionNamePunctuation = true
	arranged := syntax.Layout{
		Lines: true, Indent: "  ", Nested: true,
		Separator: ";", KeywordTerminator: ";",
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
	}
	for _, tc := range []struct{ name, src, flat, laid string }{
		{
			"the keyword form", "function f { echo kw; }",
			"function f { echo kw; }", "function f { \n  echo kw\n}",
		},
		{
			// The hybrid comes back without its parentheses, which is the
			// tree it parsed to and not a loss: ksh93 refuses this spelling
			// outright — `syntax error at line 1: '(' unexpected` — and the
			// two shells that take it run all three the same way.
			"the hybrid form", "function f() { echo both; }",
			"function f { echo both; }", "function f { \n  echo both\n}",
		},
		{
			// A name the keyword form allows and the bare one may not, so
			// the word and the name are not printed by one rule.
			"a keyword name with punctuation", "function :f { echo ok; }",
			"function :f { echo ok; }", "function :f { \n  echo ok\n}",
		},
		{
			// The control, and the half a fix that always wrote the word
			// would break: no keyword in, no keyword out.
			"the bare form", "f() { echo p; }",
			"f() { echo p; }", "f() { \n  echo p\n}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if got := syntax.Print(f); got != tc.flat {
				t.Errorf("printed %q, want %q", got, tc.flat)
			}
			if got := syntax.PrintFileWith(f, arranged); got != tc.laid {
				t.Errorf("laid out as %q, want %q", got, tc.laid)
			}
		})
	}
}

// And the keyword survives the trip as a *flag* and not only as text: what
// comes back parses to a declaration that still carries it, which is what
// the semantics axis reads. Asserted separately because a printer that wrote
// the word into a position the parser reads as something else would pass the
// text comparison above.
func TestAPrintedKeywordFunctionParsesBackAsOne(t *testing.T) {
	d := syntax.Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	for _, tc := range []struct {
		name, src string
		want      bool
	}{
		{"the keyword form", "function f { echo kw; }", true},
		{"the hybrid form", "function f() { echo both; }", true},
		{"the bare form", "f() { echo p; }", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			back, err := syntax.Parse(syntax.Print(f), d)
			if err != nil {
				t.Fatalf("printed source does not parse: %v", err)
			}
			if got := keywordOf(t, back); got != tc.want {
				t.Errorf("read back with Keyword = %v, want %v", got, tc.want)
			}
		})
	}
}

// keywordOf is the Keyword flag of the one declaration in f.
func keywordOf(t *testing.T, f *syntax.File) bool {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		t.Fatalf("not a single command: %T", f.Stmts[0].Expr)
	}
	decl, ok := pipe.Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("not a declaration: %T", pipe.Cmds[0])
	}
	return decl.Keyword
}
