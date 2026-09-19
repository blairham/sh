// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The three answers a whole *file* needs that a block does not, and the one
// boundary an arrangement may change at.
//
// An arrangement that writes a function body back has no top level: everything
// it writes is inside one block. An arrangement asked for a whole script has
// one, and the measured shape of it is three separate answers —
// Layout.StatementsShareALineOutsideADeclaration,
// Layout.FileFollowsTheSourceUnits and Layout.TrailingBlankLine — which is why
// each is its own field and each is moved on its own here.
//
// Named for the fields and not for a shell, as everything under syntax is: the
// dialect that holds these values grades them against the real thing in its own
// package.

// listing is the arrangement the three fields are exercised on: a body laid out
// over lines, with a `;` between statements and a keyword taking one before it.
func listing() syntax.Layout {
	return syntax.Layout{
		Indent:                                "    ",
		Nested:                                true,
		Lines:                                 true,
		Separator:                             ";",
		KeywordTerminator:                     ";",
		BraceOpenSuffix:                       " ",
		OutermostBraceOpensALine:              true,
		CaseHeaderSuffix:                      " ",
		FunctionHeader:                        syntax.FunctionHeaderParens,
		BraceAfterAFunctionHeaderOnItsOwnLine: true,
	}
}

func printed(t *testing.T, src string, l syntax.Layout) string {
	t.Helper()
	p := syntax.NewParser(src, syntax.Core())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	return syntax.PrintFileWith(f, l)
}

func TestStatementsShareALineOutsideADeclaration(t *testing.T) {
	for _, c := range []struct {
		name, src   string
		each, share string
	}{
		{
			// The row the field is named for: a group at file scope keeps
			// its line where one inside a declaration is opened out.
			name:  "a brace group at file scope",
			src:   "{ echo a; echo b; }",
			each:  "{ \n    echo a;\n    echo b\n}\n",
			share: "{ echo a; echo b; }\n",
		},
		{
			name:  "the same group inside a declaration",
			src:   "f() { { echo a; echo b; }; }",
			each:  "f () \n{ \n    { \n        echo a;\n        echo b\n    }\n}\n",
			share: "f () \n{ \n    { \n        echo a;\n        echo b\n    }\n}\n",
		},
		{
			// Not a depth rule and not a rule about the outermost command:
			// the `if` is opened out either way and only the group moves.
			name:  "a group inside a keyword body at file scope",
			src:   "if true; then { echo a; echo b; }; fi",
			each:  "if true; then\n    { \n        echo a;\n        echo b\n    };\nfi\n",
			share: "if true; then\n    { echo a; echo b; };\nfi\n",
		},
		{
			// The boundary is the declaration wherever it stands, so a
			// declaration inside a joined body switches back.
			name:  "a declaration inside a keyword body at file scope",
			src:   "if true; then f() { echo a; echo b; }; fi",
			each:  "if true; then\n    f () \n    { \n        echo a;\n        echo b\n    };\nfi\n",
			share: "if true; then\n    f () \n    { \n        echo a;\n        echo b\n    };\nfi\n",
		},
		{
			name:  "statements of a keyword body",
			src:   "while true; do echo a; echo b; done",
			each:  "while true; do\n    echo a;\n    echo b;\ndone\n",
			share: "while true; do\n    echo a; echo b;\ndone\n",
		},
		{
			// The control row. A keyword still opens a line of its own
			// under both values, which is the whole difference between this
			// field and turning Lines off — a fix that flattened the
			// construct as well would pass every row above and fail here.
			name:  "a keyword still opens its own line",
			src:   "if true; then echo a; fi",
			each:  "if true; then\n    echo a;\nfi\n",
			share: "if true; then\n    echo a;\nfi\n",
		},
		{
			// The second control row: a statement whose `&` already
			// terminates it takes a blank and never a separator, under
			// either value.
			name:  "a backgrounded statement is already terminated",
			src:   "f() { echo a & echo b; }",
			each:  "f () \n{ \n    echo a & echo b\n}\n",
			share: "f () \n{ \n    echo a & echo b\n}\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := listing()
			l.BackgroundKeepsTheLine = true
			l.TrailingBlankLine = true
			if got := printed(t, c.src, l); got != c.each {
				t.Errorf("a line each:\n got %q\nwant %q", got, c.each)
			}
			l.StatementsShareALineOutsideADeclaration = true
			if got := printed(t, c.src, l); got != c.share {
				t.Errorf("shared:\n got %q\nwant %q", got, c.share)
			}
		})
	}
}

func TestAFileFollowsItsOwnUnits(t *testing.T) {
	for _, c := range []struct {
		name, src string
		off, on   string
	}{
		{
			// A unit ends where the source's line did, and `;` does not end
			// one: what was written on one line stays on one.
			name: "one line is one unit",
			src:  "echo a; echo b",
			off:  "echo a; echo b",
			on:   "echo a; echo b\n",
		},
		{
			name: "two lines are two units",
			src:  "echo a\necho b",
			off:  "echo a; echo b",
			on:   "echo a\necho b\n",
		},
		{
			// A gap of any size is one blank line, which is what makes this
			// a collapse and not a copy.
			name: "a gap of any size is one blank line",
			src:  "echo a\n\n\n\necho b",
			off:  "echo a; echo b",
			on:   "echo a\n\necho b\n",
		},
		{
			// A gap before the first statement counts too, which is how a
			// comment block leaves a blank behind in a tree holding none.
			name: "a gap before the first statement",
			src:  "\n\necho a",
			off:  "echo a",
			on:   "\necho a\n",
		},
		{
			// The control row: inside a block the source's lines are not
			// consulted at all, so a fix that applied the unit rule
			// everywhere would pass the rows above and fail this one.
			name: "the source's lines do not reach inside a block",
			src:  "if true; then\necho a\necho b\nfi",
			off:  "if true; then\n    echo a; echo b;\nfi",
			on:   "if true; then\n    echo a; echo b;\nfi\n",
		},
		{
			// A here-document body is read from the lines after the command,
			// so the unit ends where the delimiter is and not where the
			// statement does. Without that the body's own lines read as a
			// gap and a blank line goes in that nobody wrote.
			name: "a here-document body is part of its unit",
			src:  "cat <<EOT\nhi\nEOT\necho after",
			off:  "cat <<EOT\nhi\nEOT\necho after",
			on:   "cat <<EOT\nhi\nEOT\n\necho after\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := listing()
			l.StatementsShareALineOutsideADeclaration = true
			l.BlankLineAfterAHereDocumentBody = true
			if got := printed(t, c.src, l); got != c.off {
				t.Errorf("off:\n got %q\nwant %q", got, c.off)
			}
			l.FileFollowsTheSourceUnits = true
			if got := printed(t, c.src, l); got != c.on {
				t.Errorf("on:\n got %q\nwant %q", got, c.on)
			}
		})
	}
}

func TestATrailingBlankLineIsItsOwnAnswer(t *testing.T) {
	l := listing()
	l.StatementsShareALineOutsideADeclaration = true
	l.FileFollowsTheSourceUnits = true
	if got, want := printed(t, "echo a", l), "echo a\n"; got != want {
		t.Errorf("without: got %q, want %q", got, want)
	}
	l.TrailingBlankLine = true
	if got, want := printed(t, "echo a", l), "echo a\n\n"; got != want {
		t.Errorf("with: got %q, want %q", got, want)
	}
	// And it is written where there was nothing at all to write, which is
	// what makes it the *end* of the output rather than a separator after
	// the last unit.
	if got, want := printed(t, "", l), "\n"; got != want {
		t.Errorf("empty: got %q, want %q", got, want)
	}
}
