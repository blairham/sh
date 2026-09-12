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
// **What that shell really does is print the source text back verbatim**,
// terminator and all: `f(){    echo     a   ;   }` lists with every one of
// those spaces, and the same definition written on a `-c` line ends with the
// `;` that followed it rather than with a newline. This engine does not keep
// the source of a function — the tree is what it has — so what is written
// here is a *layout* that reproduces that text for a definition written the
// way anybody writes one, and normalizes the spacing of one that is not.
// That divergence is deliberate and is the one thing this listing does not
// promise; see docs/spec/semantics.md.
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
