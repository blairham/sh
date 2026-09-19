// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/syntax"

// How this shell lays a function out, in the two places it does.
//
// Here rather than in syntax, because an arrangement is one shell's taste and
// nothing under syntax knows a shell. Every field was measured from the real
// thing, and they differ enough between shells to be worth saying: this one
// terminates a statement with `;` and puts `then` on the line of its `if`,
// where another terminates with nothing and gives `then` a line of its own.

// FunctionLayout is how a function is shown to a person.
func FunctionLayout() syntax.Layout {
	l := common()
	l.Indent, l.Nested = "    ", true
	// Shown, the outermost brace stands alone on its line; written into the
	// environment it does not, which is the only difference between the two.
	l.OutermostBraceOpensALine = true
	l.CommandSubstitutionIsReprinted = reprintBody(l)
	return l
}

// ScriptListingLayout is how a whole script is written back when the
// invocation asked for the program rather than a run of it.
//
// It is FunctionLayout with four answers changed, which is the finding this
// whole arrangement rests on: what the option writes is not a formatter's
// output but the *listing* layout applied to a file. Measured 2026-09-19 on
// bash 5.3.20 over 33 probe scripts, `env -i PATH=/usr/bin:/bin LC_ALL=C`.
//
// The header is the first of the four: a declaration nested inside a listed
// body is spelled with the keyword and the parentheses, and the same
// declaration written back as part of a script is spelled with the
// parentheses alone.
//
// The other three are what a file has that a function body does not — a top
// level. Outside a declaration the statements of a block share a line and a
// brace group keeps its own, where inside one each takes a line; the file's
// own units are kept, with a gap of any size between two of them collapsing
// to one blank line; and the output ends with a blank line.
func ScriptListingLayout() syntax.Layout {
	l := FunctionLayout()
	l.FunctionHeader = syntax.FunctionHeaderParens
	l.StatementsShareALineOutsideADeclaration = true
	l.FileFollowsTheSourceUnits = true
	l.TrailingBlankLine = true
	return l
}

// ExportedFunctionLayout is how a function is written into the environment for
// a child to read back.
func ExportedFunctionLayout() syntax.Layout {
	l := common()
	// One space, and the same space however deep: what goes into the
	// environment is not indented to be read.
	l.Indent, l.Nested = " ", false
	l.CommandSubstitutionIsReprinted = reprintBody(l)
	return l
}

func common() syntax.Layout {
	return syntax.Layout{
		Lines:             true,
		Separator:         ";",
		KeywordTerminator: ";",
		// `then` keeps the line of its `if`, and so does the `do` of a loop
		// over a command — but not the `do` of a loop over words, which
		// takes a line of its own. Three answers rather than one, and this
		// shell gives two of them.
		ThenOnItsOwnLine:           false,
		DoAfterCommandOnItsOwnLine: false,
		DoAfterWordsOnItsOwnLine:   true,
		BraceOpenSuffix:            " ",
		CaseHeaderSuffix:           " ",

		// What a listing says about a body beyond where its lines break,
		// all of it measured on bash 5.3.15 and bash 3.2.57 through
		// `declare -f` and `export -f`, which answer alike (#2427).
		//
		// The braces of a `${x}` are kept, because a body reprinted with
		// `$x` where `${x}` was written is a different program the moment
		// the next character continues a name. The `2>&1` a `|&` stands for
		// is written out and the operator is not — which this shell has a
		// second reason for, since `|&` arrived in bash 4 and 3.2 answers a
		// syntax error to a body it can otherwise run. A body that is not a
		// brace group is put in one, so `f() ( … )` lists as `f () \n{ ( …
		// ) \n}`. A statement after a `&` stays on the `&`'s line. And an
		// `elif` is written out as an `else` holding an `if` of its own.
		ParameterBracesAsWritten: true,
		PipeBothWrittenOut:       true,
		BodyIsAlwaysBraced:       true,
		BackgroundKeepsTheLine:   true,
		ElifWrittenAsANestedIf:   true,

		// Three more normalizations this listing makes, measured the same
		// way on bash 5.3.20 (#3196). The arithmetic `for` is the one loop
		// whose `do` takes a line of its own here, and the `1` an omitted
		// expression stands for is written out — `for ((;;))` comes back
		// `for ((1; 1; 1))`. A here-document delimiter that carries any
		// quoting is respelled with single quotes, `<<"EOT"` and `<<\EOT`
		// alike. The decoder for the third is attached where the runner is,
		// since what a `$'…'` escape comes to is this dialect's answer and
		// not the printer's.
		DoAfterArithmeticOnItsOwnLine:     true,
		EmptyArithmeticForExpressionIsOne: true,
		HereDocumentWordSingleQuoted:      true,

		// A subshell stays on one line, and the line a here-document body
		// ended is not written on again — so a body followed by anything at
		// all, the closing `}` included, has a blank line between them. The
		// other engine answers both the other way.
		SubshellBodyOnItsOwnLines:       false,
		BlankLineAfterAHereDocumentBody: true,

		// A nested declaration is respelled with both the keyword and the
		// parentheses, whichever it was written with: `inner() { … }` inside
		// a listed body comes back `function inner () ` with the brace on
		// the next line. The header of the listing *itself* is not this —
		// that is `f () `, with no keyword, and it is written by the builtin
		// rather than by the printer. Two spellings from one shell, which is
		// why the outer one is Diagnostics.FunctionListingHeader and this
		// one is here.
		// And a `$"…"` comes back as an ordinary double-quoted string:
		// measured 2026-09-19 on bash 5.3.20, `f() { echo $"hi"; }` lists as
		// `echo "hi"` under `declare -f`, under `type`, through `export -f`
		// and through the whole-script listing alike. The mark is read when
		// the word is, so a body said back after the fact has nothing left
		// to mark.
		TranslatedWordWrittenPlain: true,

		FunctionHeader:                        syntax.FunctionHeaderKeywordAndParens,
		BraceAfterAFunctionHeaderOnItsOwnLine: true,
	}
}

// How the inside of a `$( … )` is written back.
//
// A command substitution's body is held unparsed — the span is the text, and
// nothing reads it until the substitution runs — so a printer with nothing but
// the span writes the characters back. This shell does not: it re-reads the
// body and lays it out through the same listing, so `$(a >/dev/null;b)` comes
// back `$(a > /dev/null; b)` and a construct written inside one is spread over
// lines exactly as it would be outside (#3801). Measured 2026-09-19 on bash
// 5.3.20 and 3.2.57 through `declare -f` and `export -f`, which answer alike.
//
// The arrangement inside is a third one, and both halves of it are measured:
// the statements of a list follow the source's own line breaks — `$(a;b)` is
// one line and `$(a` newline `b)` is two — while a construct a reserved word
// closes is laid out, `$(if true; then echo x; fi)` coming back over three
// lines with `fi` at the margin. See syntax.Layout.KeywordBodiesTakeTheirOwnLines.
//
// It is the *listing's* arrangement rather than a second one: reprintBody is
// handed the same Layout the caller is using, so the shown form and the
// exported form each reprint with their own indent, and a substitution inside
// a substitution reprints with the arrangement of the one that holds it.

// reprintBody re-reads a command substitution's body and writes it back
// through the same arrangement.
//
// One function value however deep the nesting goes, and the knot is tied
// through the layout rather than through the closure: `l` is the copy the
// closure reads, and the line after it stores the closure *into* that copy, so
// a substitution found inside a body is reprinted by the same function with
// the same arrangement.
func reprintBody(l syntax.Layout) func(string) (string, bool) {
	// The inside of a substitution, which is the outside's arrangement with
	// the two line questions answered the other way.
	l.Lines = false
	l.KeywordBodiesTakeTheirOwnLines = true
	reprint := func(body string) (string, bool) {
		f, err := syntax.Parse(body, Dialect())
		if err != nil {
			// A body is not read until the substitution runs, so a program
			// holding one that will not parse is a program this shell lists
			// today. Writing it back as it stands is the answer; refusing to
			// print it is not.
			return "", false
		}
		return syntax.PrintFileWith(f, l), true
	}
	l.CommandSubstitutionIsReprinted = reprint
	return reprint
}
