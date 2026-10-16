// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A function body's text, printed once per declaration.
//
// zsh's `$functions` is a view: `${functions[f]}` prints f's tree every time
// it is read. F-Sy-H's lifecycle EXIT trap reads it for every autoload name
// on every highlight call, 44 names, and the autoload stub was printed twice
// per read, once to ask whether it is still a stub and once for its value
// (#6423). The printed text depends on nothing but the tree and the layout,
// and a declaration's tree is not changed once it is defined: redefining a
// function makes a new declaration rather than editing the old one.
//
// So a printed body is kept by declaration, and dropped wholesale whenever
// the layout moves: SetFunctionLayout, and the indent a listing's `-x` sets
// for the length of one call. The second is not observable today, since
// nothing read inside that call goes through the kept text and a stub's text
// is trimmed of its indent; a mutant without it survives. It is there so a
// reader added inside that window cannot keep a text in the listing's layout.
//
// A layout that asks the runner how to print a word is not kept from. Its
// answer can change under the same tree — bash's reads `$'…'` through the
// locale — so it prints every time, as before.

// bodyTextsKept bounds the table. Each distinct declaration read is one
// entry, and a script that defines functions through `eval` in a loop makes
// a new declaration every time, so the table starts again when it is full.
const bodyTextsKept = 512

// keepBodyText keeps text as fn's body under the current layout, where the
// layout's output depends on the tree alone.
func (r *Runner) keepBodyText(fn *syntax.FuncDecl, text string) {
	if !layoutIsTheTreeAlone(&r.functionLayout) {
		return
	}
	if r.bodyTexts == nil || len(r.bodyTexts) >= bodyTextsKept {
		r.bodyTexts = make(map[*syntax.FuncDecl]string, 16)
	}
	r.bodyTexts[fn] = text
}

// layoutIsTheTreeAlone reports whether l prints a tree without asking anyone
// about it, so the same tree always prints the same text.
func layoutIsTheTreeAlone(l *syntax.Layout) bool {
	return l.AnsiCQuotedWordIsItsValue == nil &&
		l.CommandSubstitutionIsReprinted == nil &&
		l.ParameterExpansionIsReprinted == nil &&
		l.ArithmeticIsReprinted == nil
}
