// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"reflect"
	"testing"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
)

// The whole corpus, printed and read back, compared as *trees*.
//
// Two properties, and the first of them is the printer's promise written as
// a test: what comes back parses to the same program. The round trip this
// replaces asserted only that the printed source parses and that printing it
// again gives the same text, which is why a group escaped into three literal
// characters passed it — `echo (v5|v6)` printed as `echo \(v5\|v6\)`, which
// parses, reprints identically, runs, exits 0 and echoes its own pattern
// (#1221).
//
// The corpus is the right adversary because nobody wrote it for a printer:
// 2,010 snippets someone wrote to pin down a *behavior*, and they are read
// here under the grammar that has to read every one of them.
//
// Comparing trees rather than text is the point. Normalisation is legitimate
// and there is a lot of it — a `case` arm's optional `(` is dropped, `;;`
// gets a space, a dup's implicit descriptor is written out — and a test that
// demanded the text back would be a test of taste. What may not change is
// the program.

// keywordFunctionsInTheCorpus is no longer a bucket of failures. It was the
// eleven cases whose declaration came back in the `name()` form because the
// printer dropped the `function` keyword — not the same declaration, since
// [syntax.FuncDecl.Keyword] is what Semantics.TypesetLocalNeedsKeywordFunction
// reads, so what came back was a program whose locals leak. The printer
// writes the word now (#1406) and they round trip like everything else.
//
// The count stays, and stays a count rather than a list of ids, because it is
// the only thing standing between "these eleven pass" and "these eleven are
// no longer reached": a case renamed or a `function` dropped from a snippet
// would leave the property untested in silence. It is asserted against the
// cases that *round trip* now rather than the ones skipped, so it can only be
// satisfied by exercising them.
//
// Twelve since #1487, whose listing case declares a keyword function on
// purpose: the round trip a `functions` listing has to survive is the same
// property this counts, asked of the builtin instead of the printer.
//
// Twenty since #1548 and #1560, which added eight keyword definitions whose
// names are written quoted: the empty string, a space, a semicolon, a pipe, a
// dollar, a pattern, a word of punctuation, and `f` — the last being the
// control that says the quoting is the question. Seven of the eight are the
// reason the printer quotes a name at all: written back bare, `function  {
// … }` is the *anonymous* function and `function a b { … }` is two names, so
// each of those is a different program, printed silently, at status 0. The
// count is what says they are walked.
//
// Twenty-four since #1680, which added four definitions giving one body
// several names. They are the sharpest cases this count has: printing only
// the first name is a program that parses, reprints identically, runs and
// exits 0 — with the rest of the script calling functions nobody defined.
//
// Twenty-five since #1576, whose case for the names-only function listing
// declares one function each way in one snippet: ksh93's `typeset +f` writes
// `f()` for the parenthesised spelling and the bare `g` for the keyword one,
// so the two forms have to stand side by side for the row to say anything.
//
// Twenty-six since #1686, whose case for the `;` between a name list and its
// body is a keyword definition written without braces — the one shape where
// the printer has to put a *newline* in front of the body, because on one
// line its words read back as more names. The bodyless case beside it is
// written inside an `eval` and is not counted here, which is deliberate:
// printed bare it would swallow the statement after it, and the grammar this
// test reads with has no empty brace group to print it as instead.
const keywordFunctionsInTheCorpus = 26

func TestPrintingTheCorpusRoundTripsToTheSameProgram(t *testing.T) {
	// The arrangement a formatter asks for, alongside the zero value that a
	// round trip asks for: both are printed, reparsed and compared, because
	// nothing says a printer that keeps the program under one layout keeps
	// it under another. Every field here is named for what it does and none
	// of them is any shell's answer — the presets live elsewhere.
	arranged := syntax.Layout{
		Lines: true, Indent: "  ", Nested: true,
		Separator: ";", KeywordTerminator: ";",
		ThenOnItsOwnLine: true, DoAfterWordsOnItsOwnLine: true,
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
		CasePatternsParenthesised: true,
	}
	for _, l := range []struct {
		name   string
		layout syntax.Layout
	}{{"as written", syntax.Layout{}}, {"arranged", arranged}} {
		t.Run(l.name, func(t *testing.T) {
			var keywordFunctions int
			for _, c := range oracle.Corpus {
				if c.SyntaxError {
					// Nothing to print: these are in the corpus because the
					// reference shells reject them.
					continue
				}
				// No skip for "not this grammar": every snippet parses under
				// the corpus dialect, which TestCorpusParses asserts, so a
				// parse failure here is a failure and not a case to pass
				// over.
				first, err := syntax.Parse(c.Snippet, oracle.Dialect())
				if err != nil {
					t.Errorf("%s: did not parse: %v\n  snippet: %s", c.ID, err, c.Snippet)
					continue
				}
				if hasKeywordFunction(first) {
					keywordFunctions++
				}
				printed := syntax.PrintFileWith(first, l.layout)

				second, err := syntax.Parse(printed, oracle.Dialect())
				if err != nil {
					t.Errorf("%s: printed source does not parse: %v\n  from: %s\n  gave: %s",
						c.ID, err, c.Snippet, printed)
					continue
				}
				if why, ok := syntax.SameProgram(first, second); !ok {
					t.Errorf("%s: printed source is a different program: %s\n  from: %s\n  gave: %s",
						c.ID, why, c.Snippet, printed)
				}
				// Printing twice is printing once: what a formatter run over
				// its own output has to leave alone.
				if again := syntax.PrintFileWith(second, l.layout); again != printed {
					t.Errorf("%s: printing is not settled:\n  from:  %s\n  once:  %s\n  twice: %s",
						c.ID, c.Snippet, printed, again)
				}
			}
			if keywordFunctions != keywordFunctionsInTheCorpus {
				t.Errorf("%d corpus cases define a function with the `function` keyword, want %d: "+
					"a case has been added or has stopped using the keyword — either way this "+
					"file has to say so, because the count is what says the keyword path above "+
					"was walked at all",
					keywordFunctions, keywordFunctionsInTheCorpus)
			}
		})
	}
}

// A pattern group is printed back as a pattern, wherever it stands.
//
// The corpus cannot reach this and says so: it is read under a grammar with
// no bare groups in it, so a leading `(` there is an operator rather than a
// pattern. These rows are the property test's discriminating cases, written
// out — each one is a group in a position the printer reaches through the
// ordinary word printer rather than through a condition's operands, which is
// where the escape was (#1221).
//
// The controls are the last block: a parenthesis the source protected stays
// protected, because there the backslash is the script's and not ours.
func TestAPatternGroupPrintsBackAsAPattern(t *testing.T) {
	bare := syntax.Core()
	bare.PatternAlternation = true
	bare.NumericRangePattern = true
	// A `(` where an argument may stand belongs to the word, which is the
	// route that puts a group at the front of one.
	bare.GlobQualifiers = true
	bare.Select = true
	bare.ShortForm = true

	behind := syntax.Core()
	behind.ExtendedPattern = true

	for _, tc := range []struct {
		name string
		d    syntax.Dialect
		src  string
	}{
		// The two rows the issue was filed on. The second is the sharp one:
		// escaped, the arm stops matching and the `case` answers the
		// opposite question at exit 0.
		{"a group as an argument", bare, `echo (v5|v6)`},
		{"a group as a case arm's whole pattern", bare, `case $k in ((a|b)) echo hit;; *) echo miss;; esac`},
		{"the same arm without the arm's own paren", bare, `case $k in (a|b)) echo hit;; *) echo miss;; esac`},
		// Not only at the front of a word, which is what the issue's title
		// said: a group mid-word takes the same escape, in an argument and
		// in an arm alike.
		{"a group behind literal text", bare, `echo a(b|c)d`},
		{"a group behind pattern text in an arm", bare, `case x in *.(zip|tgz)) :;; esac`},
		// The other positions a group standing at the front of a word
		// reaches, which are a loop's item list and `select`'s.
		{"a group in a loop's item list", bare, `for x in (v5|v6); do :; done`},
		{"a group in select's item list", bare, `select x in (v5|v6); do :; done`},
		// Blanks, a glob flag and a numeric range are all pattern text and
		// none of them is the printer's to protect.
		{"a group holding blanks", bare, `echo ( a|b )`},
		{"a glob flag", bare, `echo (#i)a`},
		{"a range inside a group", bare, `echo (5.<1->*|<6->.*)`},
		// A quoted operator inside a group keeps its quoting and the group
		// keeps its parentheses: the two halves have to come back
		// together (#1248 is the parser's half of this).
		{"a backslashed operator in a group", bare, `echo (\<)*`},
		{"a quoted operator in a group", bare, `echo ("<")*`},
		// The quantified spelling, which is the other grammar that puts a
		// group in a word and reaches the printer by the same route.
		{"a quantified group as an argument", behind, `echo @(a|b)`},
		{"a negated quantified group", behind, `echo !(a|b)`},
		{"a quantified group behind pattern text", behind, `echo *.@(zip|tgz)`},
		{"a quantified group in an arm", behind, `case x in @(a|b)) :;; esac`},
		{"a quantified group in a loop's item list", behind, `for f in *.@(c|h); do :; done`},
		// Controls. A condition's operand round-tripped before this change
		// and still does; a protected parenthesis is a literal parenthesis
		// and comes back protected; and a plain pattern is untouched.
		{"a group in a condition", bare, `[[ $k == (a|b) ]]`},
		{"a quantified group in a condition", behind, `[[ $k == @(a|b) ]]`},
		{"a protected parenthesis", bare, `echo \(v5\|v6\)`},
		{"a quoted parenthesis", bare, `echo '(' v5 ')'`},
		{"a pattern with no group in it", bare, `echo *.txt`},
		{"no pattern at all", bare, `echo hi | grep h`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, err := syntax.Parse(tc.src, tc.d)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			printed := syntax.Print(first)
			second, err := syntax.Parse(printed, tc.d)
			if err != nil {
				t.Fatalf("%s printed as %q, which does not parse: %v", tc.src, printed, err)
			}
			if why, ok := syntax.SameProgram(first, second); !ok {
				t.Errorf("%s printed as %q, which is a different program: %s", tc.src, printed, why)
			}
			if again := syntax.Print(second); again != printed {
				t.Errorf("%s printed as %q and then as %q", tc.src, printed, again)
			}
		})
	}
}

// The note is what the printer reads, and not the byte.
//
// A tree does not have to have come from a parser — this is a public printer
// — and an unquoted `(` in a word it did not read is a parenthesis with
// nothing saying it opens a group. That one is still protected, which is the
// half of the rule a fix that simply stopped escaping parentheses would get
// wrong: it would print an unbalanced word for the literal case and lose the
// distinction the flag exists to keep.
func TestAnUnmarkedParenthesisIsStillProtected(t *testing.T) {
	for _, tc := range []struct {
		name string
		span syntax.Span
		want string
	}{
		{
			"nothing says it is a group",
			syntax.Span{Kind: syntax.Literal, Value: "(a|b)", Quoting: syntax.Unquoted},
			`\(a\|b\)`,
		},
		{
			"the group says so",
			syntax.Span{Kind: syntax.Literal, Value: "(a|b)", Quoting: syntax.Unquoted, PatternGroup: true},
			`(a|b)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := syntax.PrintWord(&syntax.Word{Spans: []syntax.Span{tc.span}})
			if got != tc.want {
				t.Errorf("printed %q, want %q", got, tc.want)
			}
		})
	}
}

// hasKeywordFunction reports whether any function in f was declared with
// the `function` keyword. Counted rather than skipped now — see
// keywordFunctionsInTheCorpus.
//
// funcDeclType is this file's own, for the keyword-function scan below. The
// comparison's copies moved into the package with it (#1401); this scan did
// not, because finding a node of a type is a different question from deciding
// two trees are the same program.
var funcDeclType = reflect.TypeOf(&syntax.FuncDecl{})

// Read through reflect accessors and never through Interface: part of the
// tree is reached through an unexported embedded field — every compound
// command carries its redirections that way — and a value reached through
// one is read-only, which makes Interface panic rather than answer.
func hasKeywordFunction(f *syntax.File) bool {
	found := false
	walkNodes(reflect.ValueOf(f), func(v reflect.Value) {
		if v.Type() == funcDeclType && !v.IsNil() && v.Elem().FieldByName("Keyword").Bool() {
			found = true
		}
	})
	return found
}

// walkNodes calls fn for every non-nil pointer in the tree, which is enough
// to find a node of a given type wherever it sits.
func walkNodes(v reflect.Value, fn func(reflect.Value)) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		fn(v)
		walkNodes(v.Elem(), fn)
	case reflect.Interface:
		if !v.IsNil() {
			walkNodes(v.Elem(), fn)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			walkNodes(v.Field(i), fn)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			walkNodes(v.Index(i), fn)
		}
	}
}
