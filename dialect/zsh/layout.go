// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/syntax"

// How this shell lays a function out when it says one back — `typeset -f`,
// `declare -f`, `functions`. Measured from the real thing, and almost
// nothing agrees with the other engine that prints bodies: statements
// terminate with nothing rather than `;`, `then` and both kinds of `do` take
// lines of their own, a `case` arm keeps its parenthesis and stays on one
// line, and a level of indentation is one tab.

// FunctionLayout is how a function is shown to a person.
func FunctionLayout() syntax.Layout {
	return syntax.Layout{
		Indent: "\t",
		Nested: true,
		Lines:  true,
		// No terminator anywhere: `echo one` ends its line bare, before
		// `fi` as much as mid-block.
		Separator:         "",
		KeywordTerminator: "",
		// `then` and `do` — after words and after a command alike — each
		// start a line of their own.
		ThenOnItsOwnLine:           true,
		DoAfterCommandOnItsOwnLine: true,
		DoAfterWordsOnItsOwnLine:   true,
		// The opening brace ends its line with nothing after it, and the
		// first statement of the outermost block starts the next one.
		BraceOpenSuffix:          "",
		OutermostBraceOpensALine: true,
		// `case $1 in` ends its line; each arm is one line, pattern
		// parenthesised on both sides: `(a) echo a ;;`.
		CaseHeaderSuffix:          "",
		CaseArmsOnOneLine:         true,
		CasePatternsParenthesised: true,

		// Three things this shell agrees with the other engine about, and
		// three it does not — measured on zsh 5.9.2 through `typeset -f`
		// (#2427).
		//
		// Agreed: the braces of a `${x}` are kept, the `2>&1` a `|&` stands
		// for is written out in place of the operator, and a body that is
		// not a brace group is put in one, so `f() ( … )` lists as `f () {`
		// with the subshell inside.
		ParameterBracesAsWritten: true,
		PipeBothWrittenOut:       true,
		BodyIsAlwaysBraced:       true,

		// Not agreed, and left at their zero values so that saying so is
		// this comment's job: a statement after a `&` starts a line of its
		// own here where the other engine keeps it on the `&`'s line, and an
		// `elif` keeps its word where the other writes it out as a nested
		// `if`.
		BackgroundKeepsTheLine: false,
		ElifWrittenAsANestedIf: false,

		// A nested declaration is respelled with the parentheses and never
		// the keyword: `function inner { … }` inside a listed body comes
		// back `inner () {`, which is the same spelling this shell's own
		// listing header uses. The word carries no meaning here — both
		// bodies scope alike — so dropping it loses nothing a run can see.
		FunctionHeader:                        syntax.FunctionHeaderParens,
		BraceAfterAFunctionHeaderOnItsOwnLine: false,
	}
}
