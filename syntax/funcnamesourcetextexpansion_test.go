// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// nameIsSourceText is the core with the flag the dialect that reads a
// definition's name as the text it was written with turns on.
func nameIsSourceText() Dialect {
	d := Core()
	d.FunctionNameIsSourceText = true
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	return d
}

// A `name()` definition whose name holds an expansion is read, and the name
// is refused when the definition runs rather than while it is parsed (#1296
// fixed the keyword spelling and left this one).
//
// The stage is the whole of it, and a parse error is not a near miss:
// measured 2026-09-16 on bash 5.3.20 and bash 3.2.57, `_p_${w}() { :; }`
// answers “ `_p_${w}': not a valid identifier “, gives the definition 1 and
// carries on, where refusing to parse gives up **every line of the file after
// it** — 74 lines of one file of bash's own suite.
func TestAParenthesisedNameHoldingAnExpansionIsRead(t *testing.T) {
	d := nameIsSourceText()
	for _, tc := range []struct{ src, refused string }{
		{`_p_${w}() { :; }`, `_p_${w}`},
		{`_p_${w} () { :; }`, `_p_${w}`},
		{`x$y() { :; }`, `x$y`},
		{`x$(echo s)() { :; }`, `x$(echo s)`},
		{`sys$read() { :; }`, `sys$read`},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Errorf("%s: refused while parsing: %v", tc.src, err)
			continue
		}
		fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
		if !ok {
			t.Errorf("%s: not a function declaration", tc.src)
			continue
		}
		// The word as it was written, which is what the complaint quotes —
		// and nothing bound, so no run can call it.
		if fn.RefusedName != tc.refused {
			t.Errorf("%s: RefusedName = %q, want %q", tc.src, fn.RefusedName, tc.refused)
		}
	}
}

// The readings that are not definitions at all stay what they were: an array
// assignment is a parenthesis after a word too, and its subscript is where an
// expansion ordinarily goes.
func TestAnArrayAssignmentWithAnExpandedSubscriptIsNotADefinition(t *testing.T) {
	d := nameIsSourceText()
	d.ArrayLiteral = true
	d.ArraySubscript = true
	for _, src := range []string{`a[$i]=()`, `a[$i]=(1 2)`, `a[${i}]=()`} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if _, isFunc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); isFunc {
			t.Errorf("%s: read as a function definition", src)
		}
	}
}

// And the dialects that do not read a name as source text are untouched: a
// word carrying an expansion before `()` is still not a definition there.
func TestWithoutTheFlagAnExpandedNameIsStillRefused(t *testing.T) {
	d := Core()
	d.FunctionKeyword = true
	if _, err := Parse(`_p_${w}() { :; }`, d); err == nil {
		t.Error("parsed without FunctionNameIsSourceText, where no dialect reads it")
	}
}
