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
	}
}
