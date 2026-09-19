// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// FunctionKeywordBodyNeedsItsOwnLine reports whether a declaration written
// with the `function` keyword must break the line between its name list and
// its body.
//
// The name list after the keyword is greedy, so a body written beside it is
// read back as more *names*. Measured 2026-09-19 against zsh 5.9.2, each body
// first on the line after `function foo` and then beside it, the script then
// printing `${#functions}` and calling `foo`:
//
//	{ echo hi; }              both define one function and print `hi`
//	( echo hi )               beside it: `unknown file attribute`, status 1
//	if true; then echo hi; fi beside it: parse error near `then`, status 1
//	while/for/repeat … do …   beside it: parse error near `do`, status 1
//	case x in x) echo hi;; …  beside it: parse error near `)`, status 1
//	(( 1 )) && echo hi        beside it: `no matches found`, status 1
//	[[ -n x ]] && echo hi     beside it: `bad pattern: [[`, status 1
//	echo hi                   beside it: **three** functions, nothing run, 0
//
// The brace group is the one body that ends the list itself. Every other
// shape is either refused outright or — the last row, and the reason this is
// a rule rather than a preference — silently a different program at status 0:
// `function foo echo hi` defines `foo`, `echo` and `hi`, each with whatever
// followed as its body, and running the script prints nothing.
//
// A newline ends the name list in every dialect that has one, which is what
// makes it the separator rather than the `;` only one grammar reads there.
//
// Both printers in this tree ask this question and neither may answer it
// alone: [Print] had the guard and `internal/fmt/printer` never did, so the
// formatter rewrote that last row and nothing downstream could tell (#3746).
// It lives here for the reason [SameProgram] does — two copies of a rule the
// round trip and the formatter must agree on would eventually not agree.
//
// The parenthesised forms do not ask: `()` ends the name list itself, so
// `foo() echo hi` and `function foo() echo hi` both define one function.
// The tree does not record the hybrid's parentheses, though — see [FuncDecl]
// — so a caller working from the tree alone cannot part `function foo()` from
// `function foo`, and a line break is right for both where a blank is right
// for only one.
func FunctionKeywordBodyNeedsItsOwnLine(body Command) bool {
	_, braced := body.(*Group)
	return !braced
}
