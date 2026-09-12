// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// How far a `function` keyword's body reaches when it is not a brace group.
//
// The axis is [Dialect.FunctionKeywordBodyIsAnAndOrList]. One dialect ends
// the declaration at a brace group's `}` and lets every other body take the
// `&&` and `||` after it, which the shell shows as the *order* two commands
// come out in and this shows as the shape of the tree.

// keywordAndOrBody is the grammar this needs: the keyword, the separator that
// may stand in front of the body, the hybrid parens, and the reach itself.
func keywordAndOrBody() Dialect {
	d := Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.FunctionKeywordBodyIsOptional = true
	d.FunctionKeywordBodyIsAnAndOrList = true
	return d
}

func TestABodyThatIsNotABraceGroupTakesTheAndOrList(t *testing.T) {
	d := keywordAndOrBody()
	for _, tc := range []struct{ name, src, want string }{
		{
			// The issue's own shape: both commands are the body, so a call
			// runs them both and nothing runs where the definition stands.
			"a simple command and its continuation",
			"function a; echo X && echo Y",
			"function a { echo X && echo Y; }",
		},
		{
			"a pipeline and its continuation",
			"function a; echo X | cat && echo Y",
			"function a { echo X | cat && echo Y; }",
		},
		{
			// A compound body is not what parts the two readings — only the
			// braces are — so an `if` takes the continuation too.
			"a compound command and its continuation",
			"function a; if true; then echo X; fi && echo Y",
			"function a { if true; then echo X; fi && echo Y; }",
		},
		{
			"a negated pipeline",
			"function a; ! echo X",
			"function a { ! echo X; }",
		},
		{
			// The hybrid form goes with the keyword rather than with the
			// parentheses, which is why this is asked where the keyword was
			// written and not after the `()`.
			"the hybrid spelling",
			"function a() echo X && echo Y",
			"function a { echo X && echo Y; }",
		},
		{
			"a name list sharing the reach",
			"function a b; echo X && echo Y",
			"function a b { echo X && echo Y; }",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := d
			if tc.name == "a name list sharing the reach" {
				d.FunctionMultipleNames = true
			}
			parsesTo(t, d, tc.src, tc.want)
		})
	}
}

func TestABraceGroupBodyEndsTheDeclaration(t *testing.T) {
	d := keywordAndOrBody()
	// The one shape that stops at its own `}`: the `&&` after it continues
	// the *declaration*, so it runs where the definition stands.
	parsesTo(t, d, "function a { echo X; } && echo Y",
		"function a { echo X; } && echo Y")
	// And a `;` ends the list as it ends any and-or, so the command after it
	// is not the body either. A body of one command prints without braces,
	// which is the spelling it already had.
	parsesTo(t, d, "function a; echo X; echo Y",
		"function a\necho X; echo Y")
}

func TestTheParenthesisSpellingKeepsOneCommand(t *testing.T) {
	// `a() echo X && echo Y` is a definition of `echo X` and an `echo Y`
	// that runs where it stands — the flag is the keyword's and this is the
	// row that says so.
	d := keywordAndOrBody()
	parsesTo(t, d, "a() echo X && echo Y", "a() echo X && echo Y")
}

func TestOneCommandIsStillTheBodyItAlwaysWas(t *testing.T) {
	// A body that is one pipeline of one command is handed back bare rather
	// than wrapped, so nothing that was already a definition prints back
	// differently for having crossed this branch.
	d := keywordAndOrBody()
	fn := decl(t, "function a; echo X", d)
	if _, isGroup := fn.Body.(*Group); isGroup {
		t.Error("one command was wrapped in a group")
	}
	if _, isSimple := fn.Body.(*SimpleCmd); !isSimple {
		t.Errorf("body is %T, want a simple command", fn.Body)
	}
}

func TestWithoutTheReachTheBodyIsOneCommand(t *testing.T) {
	d := keywordAndOrBody()
	d.FunctionKeywordBodyIsAnAndOrList = false
	// The other four shells' reading, and the one this had before: the body
	// is `echo X` and `echo Y` is an and-or continuation of the declaration.
	parsesTo(t, d, "function a; echo X && echo Y",
		"function a\necho X && echo Y")
}
