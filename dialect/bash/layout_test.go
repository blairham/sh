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

		// The six below are #2427: what this shell says about a body beyond
		// where its lines break. Measured through `declare -f` and `export
		// -f` on 5.3.15, which is what the two columns are, and confirmed on
		// 3.2.57 — the two builds answer alike on all six.
		{
			// The braces of a `${x}` are kept, because the two spellings are
			// one node and are not one program the moment the next character
			// continues a name.
			"a parameter written with braces",
			`f(){ echo "${x}"; }`,
			"{  echo \"${x}\"\n}",
			"{ \n    echo \"${x}\"\n}",
		},
		{
			// And a parameter written without them stays without them, which
			// is what says the field reads the spelling rather than adding
			// braces everywhere.
			"a parameter written without braces",
			`f(){ echo "$x"; }`,
			"{  echo \"$x\"\n}",
			"{ \n    echo \"$x\"\n}",
		},
		{
			// The operator goes and the redirection it stands for is
			// written, which this build has a second reason for: `|&`
			// arrived in bash 4 and 3.2 answers a syntax error to it.
			"a pipe that carries stderr",
			`f(){ echo a |& cat; }`,
			"{  echo a 2>&1 | cat\n}",
			"{ \n    echo a 2>&1 | cat\n}",
		},
		{
			// A statement after a `&` stays on the `&`'s line.
			"a background statement with another after it",
			`f(){ echo a & echo b; }`,
			"{  echo a & echo b\n}",
			"{ \n    echo a & echo b\n}",
		},
		{
			// An `elif` is written out as an `else` holding an `if`.
			"an elif",
			`f(){ if a; then b; elif c; then d; else e; fi; }`,
			"{  if a; then\n b;\n else\n if c; then\n d;\n else\n e;\n fi;\n fi\n}",
			"{ \n    if a; then\n        b;\n    else\n        if c; then\n            d;\n" +
				"        else\n            e;\n        fi;\n    fi\n}",
		},
		{
			// A nested declaration is respelled with the keyword and the
			// parentheses, whichever it was written with, and its brace takes
			// a line of its own.
			"a nested declaration",
			`f(){ inner() { echo i; }; }`,
			"{  function inner () \n { \n echo i\n }\n}",
			"{ \n    function inner () \n    { \n        echo i\n    }\n}",
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
	// This shell's own grammar rather than the core's: `|&` is one of the
	// constructs a listing has to say back, and the core has no reading for
	// it — so a row about it could not be written against the core at all.
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("%q is not a function declaration", src)
	}
	return fn.Body
}

// A body that is not a brace group is listed as one.
//
// The table above prints a body that is already `{ … }`, so it cannot ask
// this: what changes here is the *outermost* node, which is the one thing a
// listing adds rather than arranges. `f() ( … )` and `f() if …; fi` are
// declarations this shell writes back with braces around them, and a body
// printed bare is a listing in a shape this shell never produces.
func TestABodyThatIsNotABraceGroupIsListedAsOne(t *testing.T) {
	for _, tc := range []struct {
		name, src, nested string
	}{
		{"a subshell", `f() ( echo sub )`, "{ \n    ( echo sub )\n}"},
		{"a conditional", `f() if true; then echo a; fi`, "{ \n    if true; then\n        echo a;\n    fi\n}"},
		{"a group, which is one already", `f() { echo b; }`, "{ \n    echo b\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := functionBody(t, tc.src)
			if got := syntax.PrintWith(body, bash.FunctionLayout()); got != tc.nested {
				t.Errorf("nested:\n  got  %q\n  want %q", got, tc.nested)
			}
		})
	}
}

// A here-document's operator is written tight, and the body's own lines are
// followed by a blank one.
//
// Two answers in one case because they are one measurement: this shell writes
// `cat <<XEOF` with no space and leaves the line the delimiter ended empty
// before whatever comes next — the `}` that closes the function included.
func TestAHereDocumentInAListedBody(t *testing.T) {
	body := functionBody(t, "f() {\ncat <<XEOF\nbody\nXEOF\n}")
	want := "{ \n    cat <<XEOF\nbody\nXEOF\n\n}"
	if got := syntax.PrintWith(body, bash.FunctionLayout()); got != want {
		t.Errorf("nested:\n  got  %q\n  want %q", got, want)
	}
}
