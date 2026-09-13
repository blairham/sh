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
	return l
}

// ExportedFunctionLayout is how a function is written into the environment for
// a child to read back.
func ExportedFunctionLayout() syntax.Layout {
	l := common()
	// One space, and the same space however deep: what goes into the
	// environment is not indented to be read.
	l.Indent, l.Nested = " ", false
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
		FunctionHeader:                        syntax.FunctionHeaderKeywordAndParens,
		BraceAfterAFunctionHeaderOnItsOwnLine: true,
	}
}
