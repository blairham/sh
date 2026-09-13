// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/syntax"

// How this shell lays a function out when it says one back — `functions`,
// `typeset -f`, `whence -v` with a body.
//
// It is the **compact** arrangement: no line per statement, no indentation,
// and the separators the source used. Measured 2026-09-12 on ksh93u+
// 2012-08-01 through `od -c`, from a script file so that what follows the
// definition is a newline:
//
//	f(){ :; }                  f(){ :; }\n
//	f(){ echo a; echo b; }     f(){ echo a; echo b; }\n
//	function g { typeset x=1; }  function g { typeset x=1; }\n
//	f(){ :; }; g(){ :; }       f(){ :; }\ng(){ :; }\n   (a bare `functions`)
//
// **This is the fallback and not the ordinary path.** What that shell really
// does is print the source text back verbatim, terminator and all, and this
// one does too: the parser keeps the definition's characters — see
// syntax.Dialect.FunctionDefinitionIsSourceText — and
// Diagnostics.FunctionListingIsSourceText is what writes them. This layout
// answers for a declaration that carries no text, which is one built by an
// embedder and one this shell is still waiting to read a body for, and it is
// written to be the arrangement a definition written the way anybody writes
// one comes back as (#2610).
//
// The zero Layout is already this arrangement, and saying it out loud is the
// point: a listing shape that is a default rather than a decision is one
// nobody measured.
func FunctionLayout() syntax.Layout {
	return syntax.Layout{
		// Lines is off, which turns every other field off with it: the
		// printer keeps the source's own separators, so `f(){ echo a; echo
		// b; }` stays on one line and a body written over three lines keeps
		// its newlines.
		Lines: false,
	}
}
