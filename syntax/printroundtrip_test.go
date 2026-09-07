// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
)

// The whole corpus, printed and read back, compared as *trees*.
//
// Two properties, and the first of them is the one a printer's promise is
// written as: what comes back parses to the same program. The round trip that
// stood here before this file asserted only that the printed source parses
// and that printing it again gives the same text, which is why a group could
// be escaped into three literal characters — `echo (v5|v6)` printed as
// `echo \(v5\|v6\)` — and pass. That form parses, reprints identically, runs
// and exits 0, and echoes its own pattern (#1221).
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

// caseIDsThatCannotRoundTrip is the one round-trip failure the corpus turns
// up that this file is not about, counted rather than folded in: a function
// written with the `function` keyword comes back in the `name()` form, and
// the two are not the same declaration — [syntax.FuncDecl.Keyword] is in the
// tree because one semantics axis reads it, so dropping it changes what
// `typeset` in the body does. Filed as #1406.
//
// A count and not a list of ids, so that it cannot rot in either direction:
// a new construct joining this bucket fails the test, and so does fixing the
// printer without coming back here.
const keywordFunctionsInTheCorpus = 11

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
				first, err := syntax.Parse(c.Snippet, corpusDialect())
				if err != nil {
					t.Errorf("%s: did not parse: %v\n  snippet: %s", c.ID, err, c.Snippet)
					continue
				}
				if hasKeywordFunction(first) {
					keywordFunctions++
					continue
				}
				printed := syntax.PrintFileWith(first, l.layout)

				second, err := syntax.Parse(printed, corpusDialect())
				if err != nil {
					t.Errorf("%s: printed source does not parse: %v\n  from: %s\n  gave: %s",
						c.ID, err, c.Snippet, printed)
					continue
				}
				if why, ok := sameProgram(first, second); !ok {
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
					"#1406 has been fixed, or a case has been added — either way this file has to say so",
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
			if why, ok := sameProgram(first, second); !ok {
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
// the `function` keyword. See keywordFunctionsInTheCorpus.
//
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

// What "the same program" means.
//
// Printing normalises, so tree *identity* is the wrong question: a `case`
// arm's optional `(` is dropped, `;;` gains a space, a body is re-indented,
// and none of that changes what runs. Four kinds of difference are allowed
// below and nothing else is, and each of them is either a field the tree
// keeps for a diagnostic or a normalisation the printer documents where it
// makes it.
//
// The list is deliberately short and deliberately awkward to extend: a
// printer change that requotes a character or drops a marker fails this
// test, and the fix is either the printer or a row here with the reason
// written out. That is the whole value of it to a formatter — a formatter is
// a printer whose output nobody re-reads character by character.

var (
	posType      = reflect.TypeOf(syntax.Pos{})
	wordType     = reflect.TypeOf(syntax.Word{})
	stmtType     = reflect.TypeOf(syntax.Stmt{})
	redirType    = reflect.TypeOf(syntax.Redirect{})
	funcDeclType = reflect.TypeOf(&syntax.FuncDecl{})
)

// sameProgram reports whether what was printed parses to the same program,
// and where it first differs if it does not.
func sameProgram(a, b *syntax.File) (string, bool) {
	return sameNode(reflect.ValueOf(a), reflect.ValueOf(b), "file")
}

// spellingOnly reports whether a field holds the source *spelling* of a node
// rather than anything the program does.
//
// Three of them, and each exists because something has to quote the input
// back long after it was read: a background statement's text is what a jobs
// listing shows, a redirection's is what one grammar's ambiguous-redirect
// diagnostic names, and a loop or `case` header's is what an execution trace
// writes. All three are the input's spelling by definition, so a printer that
// re-spelled the construct re-spells them with it.
func spellingOnly(t reflect.Type, name string) bool {
	if name == "Text" {
		return t == stmtType || t == redirType
	}
	// Header, on the clauses that keep one.
	return name == "Header" && t.Kind() == reflect.Struct
}

func sameNode(a, b reflect.Value, path string) (string, bool) {
	if a.Type() != b.Type() {
		return path + ": a different node", false
	}
	switch a.Type() {
	case posType:
		// Printed source has its own positions, by construction: the whole
		// point of printing is that the text is not the text that was read.
		return "", true
	case wordType:
		return sameWord(a, b, path)
	case redirType:
		return sameRedirect(a, b, path)
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() != b.IsNil() {
			return fmt.Sprintf("%s: one is absent", path), false
		}
		if a.IsNil() {
			return "", true
		}
		return sameNode(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		for i := range a.NumField() {
			f := a.Type().Field(i)
			if spellingOnly(a.Type(), f.Name) {
				continue
			}
			if why, ok := sameNode(a.Field(i), b.Field(i), path+"."+f.Name); !ok {
				return why, false
			}
		}
		return "", true
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: %d against %d", path, a.Len(), b.Len()), false
		}
		for i := range a.Len() {
			if why, ok := sameNode(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); !ok {
				return why, false
			}
		}
		return "", true
	case reflect.String:
		if a.String() != b.String() {
			return fmt.Sprintf("%s: %q against %q", path, a.String(), b.String()), false
		}
	case reflect.Bool:
		if a.Bool() != b.Bool() {
			return fmt.Sprintf("%s: %v against %v", path, a.Bool(), b.Bool()), false
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.Int() != b.Int() {
			return fmt.Sprintf("%s: %d against %d", path, a.Int(), b.Int()), false
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if a.Uint() != b.Uint() {
			return fmt.Sprintf("%s: %d against %d", path, a.Uint(), b.Uint()), false
		}
	}
	return "", true
}

// sameRedirect compares two redirections, allowing the two normalisations
// the printer's `dup` documents and measures.
//
// It writes out the descriptor being redirected where the target is one a
// reader can resolve — `>&2` comes back as `1>&2` — which adds nothing the
// source did not mean, since the operator's own default is what it adds. And
// it writes a close as `>&-` whichever operator asked for it, so `0<&-`
// comes back as `0>&-`: both close the same descriptor.
func sameRedirect(a, b reflect.Value, path string) (string, bool) {
	an, bn := a.FieldByName("N"), b.FieldByName("N")
	if an.IsNil() && !bn.IsNil() {
		want := "1"
		if syntax.Kind(a.FieldByName("Op").Uint()) == syntax.TokLessAmp {
			want = "0"
		}
		if got := literalOf(bn.Elem().FieldByName("Spans")); got != want {
			return fmt.Sprintf("%s.N: the printer put %q where the operator means %q", path, got, want), false
		}
		bn = an
	}
	if why, ok := sameNode(an, bn, path+".N"); !ok {
		return why, false
	}
	aop, bop := syntax.Kind(a.FieldByName("Op").Uint()), syntax.Kind(b.FieldByName("Op").Uint())
	closing := literalOf(a.FieldByName("Word").Elem().FieldByName("Spans")) == "-"
	if (!closing || aop != syntax.TokLessAmp || bop != syntax.TokGreatAmp) && aop != bop {
		return fmt.Sprintf("%s.Op: %s against %s", path, aop, bop), false
	}
	for i := range a.NumField() {
		f := a.Type().Field(i)
		if f.Name == "N" || f.Name == "Op" || spellingOnly(redirType, f.Name) {
			continue
		}
		if why, ok := sameNode(a.Field(i), b.Field(i), path+"."+f.Name); !ok {
			return why, false
		}
	}
	return "", true
}

// literalOf joins a span slice's values, which is the word with its quotes
// taken off and nothing else done to it.
func literalOf(spans reflect.Value) string {
	var b strings.Builder
	for i := range spans.Len() {
		b.WriteString(spans.Index(i).FieldByName("Value").String())
	}
	return b.String()
}

// sameWord compares two words by what they mean rather than span for span.
//
// Requoting *within* a word is not free — a protected `(` is a parenthesis
// where a bare one is a group, which is the whole of #1221 — but where a
// character means nothing to the matcher, protecting it changes nothing, and
// the printer does protect two such characters for reasons of its own. So a
// word is compared as its literal text, character by character, each with
// whether the source protected it, and the runs are joined across span
// boundaries: the printer may split one span into three putting quotes
// around the middle of it, and does.
func sameWord(a, b reflect.Value, path string) (string, bool) {
	x, y := atomsOf(a), atomsOf(b)
	if len(x) != len(y) {
		return fmt.Sprintf("%s: %s against %s", path, describe(x), describe(y)), false
	}
	for i := range x {
		p, q := x[i], y[i]
		switch {
		case p.literal != q.literal:
			return fmt.Sprintf("%s: %s against %s", path, describe(x), describe(y)), false
		case !p.literal:
			if why, ok := sameNode(p.span, q.span, fmt.Sprintf("%s[%d]", path, i)); !ok {
				return why, false
			}
		case p.ch != q.ch:
			return fmt.Sprintf("%s: %s against %s", path, describe(x), describe(y)), false
		case p.protected != q.protected:
			if !p.protected && freeToProtect(p.ch) {
				continue
			}
			what := "protected"
			if p.protected {
				what = "left bare"
			}
			return fmt.Sprintf("%s: %q was %s: %s against %s",
				path, string(p.ch), what, describe(x), describe(y)), false
		}
	}
	return "", true
}

// atom is one character of a word's literal text, or one substitution in it.
type atom struct {
	literal   bool
	ch        byte
	protected bool
	span      reflect.Value
}

func atomsOf(w reflect.Value) []atom {
	var out []atom
	spans := w.FieldByName("Spans")
	for i := range spans.Len() {
		s := spans.Index(i)
		if syntax.SpanKind(s.FieldByName("Kind").Uint()) != syntax.Literal {
			out = append(out, atom{span: s})
			continue
		}
		protected := syntax.Quoting(s.FieldByName("Quoting").Uint()) != syntax.Unquoted
		v := s.FieldByName("Value").String()
		for j := 0; j < len(v); j++ {
			out = append(out, atom{literal: true, ch: v[j], protected: protected})
		}
	}
	return out
}

// freeToProtect are the characters a print may protect without changing the
// program. Two of them, both reached by the corpus, and the set is this
// small on purpose.
//
// A literal dollar, which the printer backslashes because bare it could open
// something in a grammar that has a construct this one does not: `$"a"` is a
// dollar and a quoted string under one flag and a translatable string under
// another. Neither reading is a pattern, so `$` and `\$` match the same
// subject.
//
// And a newline, which the printer puts in single quotes rather than behind
// a backslash — a backslash before a newline is a line continuation and the
// next read removes it, so that is the one protection that would lose the
// character. Reached by a `case` arm whose pattern list spans a line.
//
// One direction only. A protection *removed* is a change even for a
// character that means nothing to a matcher: an unquoted here-document
// delimiter is a body that expands, and its letters mean nothing to anything
// else.
func freeToProtect(c byte) bool { return c == '$' || c == '\n' }

// describe renders a word's atoms for a failure message, marking a protected
// character with a leading backslash so the difference is legible.
func describe(atoms []atom) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, a := range atoms {
		if !a.literal {
			b.WriteString("${…}")
			continue
		}
		if a.protected {
			b.WriteByte('\\')
		}
		if a.ch == '\n' {
			b.WriteString("\\n")
			continue
		}
		b.WriteByte(a.ch)
	}
	b.WriteByte('"')
	return b.String()
}
