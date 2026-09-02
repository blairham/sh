// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// This shell's two arrangements, measured from it: what it shows a person and
// what it writes into the environment.
//
// Here rather than beside the printer, because an arrangement is one shell's
// taste — the printer knows how to apply one and decides none of it, which
// syntax's own test proves by writing a different shell's.
//
// A table rather than a property because the rules are per construct and not
// uniform: a `then` stays where it is and a `do` moves, a body closed by a
// keyword takes a `;` and one closed by a brace does not.
func TestTheTwoArrangements(t *testing.T) {
	flat := bash.ExportedFunctionLayout()
	nested := bash.FunctionLayout()

	for _, tc := range []struct {
		name, src    string
		flat, nested string
	}{
		{
			"one command",
			`f(){ echo hi; }`,
			"{  echo hi\n}",
			"{ \n    echo hi\n}",
		},
		{
			// `;` between, and none before the brace.
			"two commands",
			`f(){ echo a; echo b; }`,
			"{  echo a;\n echo b\n}",
			"{ \n    echo a;\n    echo b\n}",
		},
		{
			// `then` stays on the line of its `if`, and the body's last
			// statement takes a `;` because a keyword closes it.
			"a conditional",
			`f(){ if true; then echo y; fi; }`,
			"{  if true; then\n echo y;\n fi\n}",
			"{ \n    if true; then\n        echo y;\n    fi\n}",
		},
		{
			// `do` moves to a line of its own after a `for`.
			"a loop over words",
			`f(){ for i in 1 2; do echo $i; done; }`,
			"{  for i in 1 2;\n do\n echo $i;\n done\n}",
			"{ \n    for i in 1 2;\n    do\n        echo $i;\n    done\n}",
		},
		{
			// And stays put after a `while`, whose header is a command
			// rather than a word list.
			"a loop over a command",
			`f(){ while true; do break; done; }`,
			"{  while true; do\n break;\n done\n}",
			"{ \n    while true; do\n        break;\n    done\n}",
		},
		{
			// An arm's body is closed by `;;` rather than a keyword, so its
			// last statement takes no `;`.
			"arms",
			`f(){ case x in a) echo A;; esac; }`,
			"{  case x in \n a)\n echo A\n ;;\n esac\n}",
			"{ \n    case x in \n        a)\n            echo A\n        ;;\n    esac\n}",
		},
		{
			// The outermost brace opens on the brace's line in one and on
			// its own in the other; a brace inside one opens on its own line
			// in both.
			"a brace inside a brace",
			`f(){ { echo g; }; }`,
			"{  { \n echo g\n }\n}",
			"{ \n    { \n        echo g\n    }\n}",
		},
		{
			// Spaced, and inline: a subshell is not a block here.
			"a subshell",
			`f(){ ( echo s ); }`,
			"{  ( echo s )\n}",
			"{ \n    ( echo s )\n}",
		},
		{
			"nesting deepens only in one of them",
			`f(){ if true; then if false; then echo deep; fi; fi; }`,
			"{  if true; then\n if false; then\n echo deep;\n fi;\n fi\n}",
			"{ \n    if true; then\n        if false; then\n            echo deep;\n        fi;\n    fi\n}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := functionBody(t, tc.src)
			if got := syntax.PrintWith(body, flat); got != tc.flat {
				t.Errorf("flat:\n  got  %q\n  want %q", got, tc.flat)
			}
			if got := syntax.PrintWith(body, nested); got != tc.nested {
				t.Errorf("nested:\n  got  %q\n  want %q", got, tc.nested)
			}
		})
	}
}

func functionBody(t *testing.T, src string) syntax.Command {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("%q is not a function declaration", src)
	}
	return fn.Body
}
