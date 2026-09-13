// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell's arrangement for a body it says back, measured from it.
//
// Here rather than beside the printer, because an arrangement is one shell's
// taste — the printer knows how to apply one and decides none of it.
//
// The rows are the constructs #2427 is about, and the point of having them in
// two dialect packages is that six of them **split**: this shell and the
// other engine answer the same question differently from the same tree, which
// is why each is a field on syntax.Layout and not a rule in the printer.
// Measured on zsh 5.9.2 through `typeset -f`, 2026-09-12.
func TestTheArrangementOfAListedBody(t *testing.T) {
	l := zsh.FunctionLayout()
	for _, tc := range []struct {
		name, src, want, why string
	}{
		{
			"a parameter written with braces",
			`f(){ echo "${x}"; }`,
			"{\n\techo \"${x}\"\n}",
			"agreed with the other engine: the spelling that was read is the spelling written back",
		},
		{
			"a parameter written without braces",
			`f(){ echo "$x"; }`,
			"{\n\techo \"$x\"\n}",
			"the control, which says the field reads the spelling rather than adding braces",
		},
		{
			"a pipe that carries stderr",
			`f(){ echo a |& cat; }`,
			"{\n\techo a 2>&1 | cat\n}",
			"agreed: the operator goes and the redirection it stands for is written",
		},
		{
			"a background statement with another after it",
			`f(){ echo a & echo b; }`,
			"{\n\techo a &\n\techo b\n}",
			"split: this shell gives the next statement a line, the other keeps it on the `&`'s",
		},
		{
			"an elif",
			`f(){ if a; then b; elif c; then d; else e; fi; }`,
			"{\n\tif a\n\tthen\n\t\tb\n\telif c\n\tthen\n\t\td\n\telse\n\t\te\n\tfi\n}",
			"split: this shell keeps the word where the other writes out a nested `if`",
		},
		{
			"a nested declaration written with parentheses",
			`f(){ inner() { echo i; }; }`,
			"{\n\tinner () {\n\t\techo i\n\t}\n}",
			"split: a blank before the parentheses and no keyword, where the other writes both",
		},
		{
			"a nested declaration written with the keyword",
			`f(){ function inner { echo i; }; }`,
			"{\n\tinner () {\n\t\techo i\n\t}\n}",
			"the same spelling from the other declaration, which is what says this respells",
		},
		{
			"a subshell",
			`f(){ ( exit 1 ); }`,
			"{\n\t(\n\t\texit 1\n\t)\n}",
			"split: a subshell gets the shape a brace group gets here and stays inline there",
		},
		{
			"a here-document",
			"f() {\ncat <<XEOF\nbody\nXEOF\n}",
			"{\n\tcat <<XEOF\nbody\nXEOF\n}",
			"split on the blank line: the delimiter's line is written on again here and left empty there",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := listedBody(t, tc.src)
			if got := syntax.PrintWith(body, l); got != tc.want {
				t.Errorf("got  %q\nwant %q\n%s", got, tc.want, tc.why)
			}
		})
	}
}

// A body that is not a brace group is listed as one, which is the seventh
// question and the one the table above cannot ask: what changes is the
// outermost node rather than how a block is arranged.
func TestABodyThatIsNotABraceGroupIsListedAsOne(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"a subshell", `f() ( echo sub )`, "{\n\t(\n\t\techo sub\n\t)\n}"},
		{"a conditional", `f() if true; then echo a; fi`, "{\n\tif true\n\tthen\n\t\techo a\n\tfi\n}"},
		{"a group, which is one already", `f() { echo b; }`, "{\n\techo b\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := listedBody(t, tc.src)
			if got := syntax.PrintWith(body, zsh.FunctionLayout()); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// listedBody is the body of the declaration a source's first statement is,
// read under this shell's own grammar — `|&` is one of the constructs a
// listing has to say back and the core has no reading for it.
func listedBody(t *testing.T, src string) syntax.Command {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("%q is not a function declaration", src)
	}
	return fn.Body
}
