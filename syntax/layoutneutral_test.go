// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Two arrangements this package does not know, built entirely out of the
// fields — which is the whole reason the fields exist.
//
// The first is measured from one shell and the second from another, and they
// disagree at almost every junction: one terminates a statement with `;` and
// the other with nothing, one keeps `then` on the line of its `if` and the
// other gives it a line, one indents with spaces and the other with a tab,
// one writes case arms down the page and the other across it.
//
// If this package decided any of that, only one of these could be written.
// That is the test: not that either is right, but that both are sayable.
func TestAnArrangementIsTheCallersAndNotThisPackages(t *testing.T) {
	// Four spaces, `;` terminators, `then` kept with its `if`, the `do` of a
	// loop over words on its own line.
	first := syntax.Layout{
		Indent: "    ", Nested: true, Lines: true,
		Separator: ";", KeywordTerminator: ";",
		DoAfterWordsOnItsOwnLine: true,
		BraceOpenSuffix:          " ",
		OutermostBraceOpensALine: true,
		CaseHeaderSuffix:         " ",
	}
	// A tab, no terminators at all, every body-opening keyword on its own
	// line, arms across the page with their patterns parenthesised.
	second := syntax.Layout{
		Indent: "\t", Nested: true, Lines: true,
		ThenOnItsOwnLine:           true,
		DoAfterWordsOnItsOwnLine:   true,
		DoAfterCommandOnItsOwnLine: true,
		CaseArmsOnOneLine:          true,
		CasePatternsParenthesised:  true,
		// The brace opens a line here too, and with nothing after it.
		OutermostBraceOpensALine: true,
	}

	for _, tc := range []struct {
		name, src     string
		first, second string
	}{
		{
			"a conditional",
			`f(){ if true; then echo y; fi; }`,
			"{ \n    if true; then\n        echo y;\n    fi\n}",
			"{\n\tif true\n\tthen\n\t\techo y\n\tfi\n}",
		},
		{
			"a loop over a command",
			`f(){ while true; do break; done; }`,
			"{ \n    while true; do\n        break;\n    done\n}",
			"{\n\twhile true\n\tdo\n\t\tbreak\n\tdone\n}",
		},
		{
			"two commands",
			`f(){ echo a; echo b; }`,
			"{ \n    echo a;\n    echo b\n}",
			"{\n\techo a\n\techo b\n}",
		},
		{
			"arms",
			`f(){ case x in a) echo A;; esac; }`,
			"{ \n    case x in \n        a)\n            echo A\n        ;;\n    esac\n}",
			"{\n\tcase x in\n\t\t(a) echo A ;;\n\tesac\n}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := functionBody(t, tc.src)
			if got := syntax.PrintWith(body, first); got != tc.first {
				t.Errorf("first:\n  got  %q\n  want %q", got, tc.first)
			}
			if got := syntax.PrintWith(body, second); got != tc.second {
				t.Errorf("second:\n  got  %q\n  want %q", got, tc.second)
			}
		})
	}
}

// Two knobs that no shell measured so far tells apart, kept apart anyway.
//
// Every arrangement seen indents its blocks *and* gives the outermost brace a
// line of its own, or does neither — so one field could stand for both today.
// Tying them together would be writing a coincidence into the type, and the
// next shell to want a nested indent with the brace on its own line would
// have to unpick it. They are separate, and this says so.
func TestIndentingAndTheOutermostBraceAreSeparateQuestions(t *testing.T) {
	body := functionBody(t, "f(){ if true; then echo y; fi; }")

	nestedInline := syntax.Layout{
		Indent: "  ", Nested: true, Lines: true, Separator: ";",
		BraceOpenSuffix: " ",
		// Nested, and the brace keeps its own line's company.
		OutermostBraceOpensALine: false,
	}
	if got, want := syntax.PrintWith(body, nestedInline), "{   if true; then\n    echo y\n  fi\n}"; got != want {
		t.Errorf("nested with the brace inline:\n  got  %q\n  want %q", got, want)
	}

	flatOpened := syntax.Layout{
		Indent: " ", Nested: false, Lines: true, Separator: ";",
		BraceOpenSuffix:          " ",
		OutermostBraceOpensALine: true,
	}
	if got, want := syntax.PrintWith(body, flatOpened), "{ \n if true; then\n echo y\n fi\n}"; got != want {
		t.Errorf("flat with the brace on its own line:\n  got  %q\n  want %q", got, want)
	}
}

// And asking for nothing gets an arrangement that belongs to nobody: the
// source's own line structure, which is what a round trip wants and what a
// caller that has not chosen should get rather than somebody's house style.
func TestTheZeroArrangementIsNobodys(t *testing.T) {
	body := functionBody(t, "f(){ if true; then echo y; fi; }")
	if got, want := syntax.PrintWith(body, syntax.Layout{}), "{ if true; then echo y; fi; }"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// functionBody returns the body of the function src declares.
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
