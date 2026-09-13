// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The fields a *listing* arrangement adds, one at a time, on and off.
//
// A listing is what `declare -f` and `export -f` produce: a body said back as
// text somebody else parses. Six questions about it have two answers among
// the engines that have the builtin, and each is a field on [syntax.Layout]
// rather than a conditional here — this package decides none of them, which
// is what the `off` column asserts.
//
// The `off` column is the control and is half the rows, deliberately. A test
// that only ever asked for the field on would pass against a printer that
// ignored the field and wrote the arrangement unconditionally, which is the
// same defect one level along: a listing shape nobody chose.
func TestTheFieldsOfAListingArrangement(t *testing.T) {
	// The common half, written out rather than taken from a dialect: what is
	// under test is the arrangement, and which shell arranges things this
	// way is not this package's business.
	base := syntax.Layout{
		Indent: "    ", Nested: true, Lines: true,
		Separator: ";", KeywordTerminator: ";",
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
	}
	with := func(set func(*syntax.Layout)) syntax.Layout {
		l := base
		set(&l)
		return l
	}

	for _, tc := range []struct {
		name     string
		src      string
		layout   syntax.Layout
		on, off  string
		roundTri bool
		why      string
	}{
		{
			name:   "the braces of a parameter that was written with them",
			src:    `f() { echo "${x}"; }`,
			layout: with(func(l *syntax.Layout) { l.ParameterBracesAsWritten = true }),
			on:     "{ \n    echo \"${x}\"\n}",
			off:    "{ \n    echo \"$x\"\n}",
			why: "the two spellings are one node to everything that expands them and " +
				"are not one text, and Span.Bare is what records which was read",
			roundTri: true,
		},
		{
			name:   "a name that needs its braces keeps them either way",
			src:    `f() { echo "${x}y"; }`,
			layout: with(func(l *syntax.Layout) { l.ParameterBracesAsWritten = true }),
			on:     "{ \n    echo \"${x}y\"\n}",
			off:    "{ \n    echo \"${x}y\"\n}",
			why: "the control for the field: unbraced this is the parameter `xy`, so " +
				"the braces are already the printer's own answer and the field changes nothing",
			roundTri: true,
		},
		{
			name:   "the redirection a stderr pipe stands for",
			src:    `f() { echo a |& cat; }`,
			layout: with(func(l *syntax.Layout) { l.PipeBothWrittenOut = true }),
			on:     "{ \n    echo a 2>&1 | cat\n}",
			off:    "{ \n    echo a |& cat\n}",
			why: "the redirection is in the tree either way — Redirect.PipeBoth — so the " +
				"field decides which of the two a reader is shown",
		},
		{
			name:   "a pipeline with no stderr in it",
			src:    `f() { echo a | cat; }`,
			layout: with(func(l *syntax.Layout) { l.PipeBothWrittenOut = true }),
			on:     "{ \n    echo a | cat\n}",
			off:    "{ \n    echo a | cat\n}",
			why:    "the control: a plain pipe is a plain pipe under either answer",

			roundTri: true,
		},
		{
			name:   "a body that is not a brace group",
			src:    `f() ( echo sub )`,
			layout: with(func(l *syntax.Layout) { l.BodyIsAlwaysBraced = true }),
			on:     "{ \n    ( echo sub )\n}",
			off:    "( echo sub )",
			why: "a listing writes the shape a listing always has rather than the shape " +
				"the author wrote, and the braces are a node the source did not have",
		},
		{
			name:   "a body that is one already",
			src:    `f() { echo b; }`,
			layout: with(func(l *syntax.Layout) { l.BodyIsAlwaysBraced = true }),
			on:     "{ \n    echo b\n}",
			off:    "{ \n    echo b\n}",
			why:    "the control: nothing is added around a group, so the field cannot double them",

			roundTri: true,
		},
		{
			name:   "a subshell inside a body",
			src:    `f() { ( exit 1 ); }`,
			layout: with(func(l *syntax.Layout) { l.SubshellBodyOnItsOwnLines = true }),
			on:     "{ \n    (\n        exit 1\n    )\n}",
			off:    "{ \n    ( exit 1 )\n}",
			why:    "the shape a brace group gets, given to a subshell — or not",

			roundTri: true,
		},
		{
			name:   "the statement after a background one",
			src:    `f() { echo a & echo b; }`,
			layout: with(func(l *syntax.Layout) { l.BackgroundKeepsTheLine = true }),
			on:     "{ \n    echo a & echo b\n}",
			off:    "{ \n    echo a &\n    echo b\n}",
			why: "the `&` is the terminator, so both readings are the same two statements " +
				"and the field is only about where the line breaks",

			roundTri: true,
		},
		{
			name:   "an elif",
			src:    `f() { if a; then b; elif c; then d; else e; fi; }`,
			layout: with(func(l *syntax.Layout) { l.ElifWrittenAsANestedIf = true }),
			on: "{ \n    if a; then\n        b;\n    else\n        if c; then\n" +
				"            d;\n        else\n            e;\n        fi;\n    fi\n}",
			off: "{ \n    if a; then\n        b;\n    elif c; then\n        d;\n" +
				"    else\n        e;\n    fi\n}",
			why: "the one field here a round trip cannot survive: the two are the same " +
				"program and not the same tree, which is why it is asked for",
		},
		{
			name:   "an if with no elif in it",
			src:    `f() { if a; then b; else e; fi; }`,
			layout: with(func(l *syntax.Layout) { l.ElifWrittenAsANestedIf = true }),
			on:     "{ \n    if a; then\n        b;\n    else\n        e;\n    fi\n}",
			off:    "{ \n    if a; then\n        b;\n    else\n        e;\n    fi\n}",
			why:    "the control: with no `elif` to expand there is nothing for the field to do",

			roundTri: true,
		},
		{
			name:   "a here-document body and what follows it",
			src:    "f() {\ncat <<E\nbody\nE\necho after\n}",
			layout: with(func(l *syntax.Layout) { l.BlankLineAfterAHereDocumentBody = true }),
			on:     "{ \n    cat <<E\nbody\nE\n\n    echo after\n}",
			off:    "{ \n    cat <<E\nbody\nE\n    echo after\n}",
			why: "the delimiter's line is already ended, so the newline that opens the " +
				"next one leaves a blank line the source never had",

			roundTri: true,
		},
		{
			name:   "a body with no here-document in it",
			src:    "f() {\necho one\necho after\n}",
			layout: with(func(l *syntax.Layout) { l.BlankLineAfterAHereDocumentBody = true }),
			on:     "{ \n    echo one;\n    echo after\n}",
			off:    "{ \n    echo one;\n    echo after\n}",
			why:    "the control: nothing else ends a line, so nothing else can gain a blank one",

			roundTri: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := listedBody(t, tc.src)
			if got := syntax.PrintWith(body, tc.layout); got != tc.on {
				t.Errorf("field on:\n  got  %q\n  want %q\n%s", got, tc.on, tc.why)
			}
			if got := syntax.PrintWith(body, base); got != tc.off {
				t.Errorf("field off:\n  got  %q\n  want %q\n%s", got, tc.off, tc.why)
			}
			// What comes back has to be readable whichever answer was
			// chosen — a listing nobody can parse is the failure the whole
			// exercise is about — and where the field is a pure re-spelling
			// it has to read back to the same program too.
			printed := syntax.PrintWith(body, tc.layout)
			if _, err := syntax.Parse(printed, listingDialect()); err != nil {
				t.Fatalf("the arrangement does not parse: %v\n%s", err, printed)
			}
			if !tc.roundTri {
				return
			}
			first, err := syntax.Parse("f() "+printed, listingDialect())
			if err != nil {
				t.Fatalf("re-read as a declaration: %v", err)
			}
			original, err := syntax.Parse(tc.src, listingDialect())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if why, ok := syntax.SameProgram(original, first); !ok {
				t.Errorf("the arrangement changed the program: %s\n  from: %s\n  gave: %s",
					why, tc.src, printed)
			}
		})
	}
}

// The header of a declaration, which is the third answer with two of its own.
//
// Separate from the table above because the question is not about a body: it
// is which of `function f`, `f ()` and `function f ()` is written back, and
// the tree carries the distinction on FuncDecl.Keyword for a reason — one
// dialect scopes a `typeset` by it.
func TestTheHeaderOfADeclarationIsRespelledOnlyWhenAsked(t *testing.T) {
	base := syntax.Layout{
		Indent: "    ", Nested: true, Lines: true,
		Separator: ";", KeywordTerminator: ";",
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
	}
	keywordAndParens := base
	keywordAndParens.FunctionHeader = syntax.FunctionHeaderKeywordAndParens
	keywordAndParens.BraceAfterAFunctionHeaderOnItsOwnLine = true
	parens := base
	parens.FunctionHeader = syntax.FunctionHeaderParens

	for _, tc := range []struct {
		name                        string
		src                         string
		asWritten, bothForms, paren string
		why                         string
	}{
		{
			name:      "written with the keyword",
			src:       `outer() { function inner { echo i; }; }`,
			asWritten: "{ \n    function inner { \n        echo i\n    }\n}",
			bothForms: "{ \n    function inner () \n    { \n        echo i\n    }\n}",
			paren:     "{ \n    inner () { \n        echo i\n    }\n}",
			why: "the default writes the word back because one dialect's locality is " +
				"decided by it; a listing that respells says which spelling it wants",
		},
		{
			name:      "written with parentheses",
			src:       `outer() { inner() { echo i; }; }`,
			asWritten: "{ \n    inner() { \n        echo i\n    }\n}",
			bothForms: "{ \n    function inner () \n    { \n        echo i\n    }\n}",
			paren:     "{ \n    inner () { \n        echo i\n    }\n}",
			why: "the same two spellings reached from the other declaration, which is " +
				"what says the field respells rather than preserves",
		},
		{
			name:      "the hybrid",
			src:       `outer() { function inner() { echo i; }; }`,
			asWritten: "{ \n    function inner { \n        echo i\n    }\n}",
			bothForms: "{ \n    function inner () \n    { \n        echo i\n    }\n}",
			paren:     "{ \n    inner () { \n        echo i\n    }\n}",
			why:       "the hybrid parses to the keyword declaration, so it prints as one",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := listedBody(t, tc.src)
			for _, want := range []struct {
				name   string
				layout syntax.Layout
				text   string
			}{
				{"as written", base, tc.asWritten},
				{"keyword and parens", keywordAndParens, tc.bothForms},
				{"parens", parens, tc.paren},
			} {
				got := syntax.PrintWith(body, want.layout)
				if got != want.text {
					t.Errorf("%s:\n  got  %q\n  want %q\n%s", want.name, got, want.text, tc.why)
				}
				if _, err := syntax.Parse(got, listingDialect()); err != nil {
					t.Errorf("%s does not parse: %v\n%s", want.name, err, got)
				}
			}
		})
	}
}

// The keyword survives the round trip under the default and under the hybrid
// spelling, and does not under the third.
//
// This is the property #1406 is about, asked of the new field: a declaration
// respelled `f ()` is a *different* declaration where `typeset` scopes by the
// word, so the loss is real and the field is what consents to it.
func TestRespellingAHeaderKeepsTheKeywordInTwoOfThreeAnswers(t *testing.T) {
	base := syntax.Layout{Lines: true, BraceOpenSuffix: " ", Separator: ";"}
	for _, tc := range []struct {
		name    string
		header  syntax.FunctionHeader
		keyword bool
	}{
		{"as written", syntax.FunctionHeaderAsWritten, true},
		{"keyword and parens", syntax.FunctionHeaderKeywordAndParens, true},
		{"parens", syntax.FunctionHeaderParens, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := base
			l.FunctionHeader = tc.header
			f, err := syntax.Parse(`function g { echo i; }`, listingDialect())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := syntax.PrintFileWith(f, l)
			again, err := syntax.Parse(printed, listingDialect())
			if err != nil {
				t.Fatalf("%q does not parse: %v", printed, err)
			}
			decl, ok := again.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
			if !ok {
				t.Fatalf("%q is not a declaration", printed)
			}
			if decl.Keyword != tc.keyword {
				t.Errorf("%q re-reads with Keyword=%v, want %v: the word is what "+
					"Semantics.TypesetLocalNeedsKeywordFunction reads",
					printed, decl.Keyword, tc.keyword)
			}
		})
	}
}

// The operator of a here-document is written tight against its delimiter, and
// the two delimiters that would make a longer operator out of the pair are
// held off it.
//
// `<<-E` is the tab-stripping document and `<<<E` is a herestring: either one
// is a different construct reading a different body, and both print at status
// 0. The rows that keep the space are the discriminating ones — a printer
// that wrote every delimiter tight passes the first four.
func TestAHereDocumentOperatorIsWrittenTightExceptWhereThatMakesAnotherOperator(t *testing.T) {
	for _, tc := range []struct {
		name, src, want, why string
	}{
		{
			"the ordinary delimiter", "cat <<E\nb\nE\n", "cat <<E\nb\nE\n",
			"no shell that says a body back writes the space",
		},
		{
			"a quoted delimiter", "cat <<'E'\nb\nE\n", "cat <<'E'\nb\nE\n",
			"the quoting says whether the body expands and nothing about the spacing",
		},
		{
			"the tab-stripping operator", "cat <<-E\n\tb\n\tE\n", "cat <<-E\nb\nE\n",
			"`<<-` extends into nothing, so its delimiter is always tight",
		},
		{
			"a delimiter that starts with a dash", "cat << -E\nb\n-E\n", "cat << -E\nb\n-E\n",
			"tight this is `<<-E`, the tab-stripping document, which reads the body by another rule",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, listingDialect())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			got := syntax.Print(f)
			if got != tc.want {
				t.Errorf("Print(%q) = %q, want %q\n%s", tc.src, got, tc.want, tc.why)
			}
			again, err := syntax.Parse(got, listingDialect())
			if err != nil {
				t.Fatalf("%q does not parse: %v", got, err)
			}
			if why, ok := syntax.SameProgram(f, again); !ok {
				t.Errorf("%q is a different program: %s", got, why)
			}
		})
	}
}

// listingDialect is the core grammar with the two constructs these rows need
// that it does not carry: the `|&` pipe and the hybrid `function f()` header.
//
// Named for the flags rather than for a shell, which is what a test in this
// package may say: a dialect package is where "bash has this" is written down.
func listingDialect() syntax.Dialect {
	d := syntax.Core()
	d.PipeBothStreams = true
	d.FunctionKeywordParens = true
	return d
}

// listedBody is the body of the declaration a source's first statement is.
func listedBody(t *testing.T, src string) syntax.Command {
	t.Helper()
	f, err := syntax.Parse(src, listingDialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("%q is not a function declaration", src)
	}
	return fn.Body
}
