// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document body ends the line it was written on, and the punctuation
// that would have followed it goes — the statement's own, and the **next**
// separator after that.
//
// Named by the arrangement rather than by a shell: the rows below are an
// engine that separates statements with a `;` and terminates a keyword body
// with one, which is the only arrangement where the difference is visible at
// all. See printer.oweASkippedSeparator for the measurement the rows come
// from.
func heredocLayout() syntax.Layout {
	return syntax.Layout{
		Indent:            "    ",
		Lines:             true,
		Separator:         ";",
		KeywordTerminator: ";",
		BraceOpenSuffix:   " ",
	}
}

func printedWithHeredocLayout(t *testing.T, src string) string {
	t.Helper()
	d := syntax.Core()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return syntax.PrintFileWith(f, heredocLayout())
}

func TestAHereDocumentBodyDropsTwoSeparators(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		// want is the punctuation each statement of the printed text ends
		// with, read off the lines that carry one.
		absent  []string
		present []string
	}{
		{
			name:    "a flat list",
			src:     "cat <<E\nx\nE\necho a; echo b; echo c",
			absent:  []string{"echo a;"},
			present: []string{"echo b;"},
		},
		{
			name:    "a statement before it keeps its own",
			src:     "echo z; cat <<E\nx\nE\necho a; echo b",
			present: []string{"echo z;"},
			absent:  []string{"echo a;"},
		},
		{
			// A keyword closes the body, so the skip is carried out of the
			// loop and lands on the `done`.
			name:    "out of a loop",
			src:     "for i in 1; do cat <<E\nx\nE\ndone\necho a; echo b",
			absent:  []string{"done;"},
			present: []string{"echo a;"},
		},
		{
			// The `;` that closes a keyword body is a terminator: it neither
			// takes the skip nor consumes it, so the pair has a whole
			// statement between them.
			name:    "a terminator between the two",
			src:     "for i in 1; do cat <<E\nx\nE\necho q; done\necho a; echo b",
			present: []string{"echo q;", "echo a;"},
			absent:  []string{"done;"},
		},
		{
			// A bracket closes this one, and that cancels the skip.
			name:    "a subshell keeps its separator",
			src:     "( cat <<E\nx\nE\n)\necho a",
			present: []string{");"},
		},
		{
			name:    "and so does a brace group",
			src:     "{ cat <<E\nx\nE\n}\necho a",
			present: []string{"};"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := printedWithHeredocLayout(t, c.src)
			lines := strings.Split(out, "\n")
			has := func(want string) bool {
				for _, l := range lines {
					if strings.TrimSpace(l) == want {
						return true
					}
				}
				return false
			}
			for _, w := range c.present {
				if !has(w) {
					t.Errorf("printed %q, want a line %q in it", out, w)
				}
			}
			for _, w := range c.absent {
				if has(w) {
					t.Errorf("printed %q, want no line %q in it", out, w)
				}
			}
		})
	}
}

// The default name a coprocess over a compound command is listed with, and
// the two shapes that must not take one.
func TestACoprocessOverACompoundCommandTakesTheDefaultName(t *testing.T) {
	l := syntax.Layout{CoprocessDefaultName: "COPROC"}
	d := syntax.Core()
	d.Coproc, d.CoprocName = true, true
	for _, c := range []struct{ src, want string }{
		{"coproc ( : )", "coproc COPROC ( : )"},
		// A simple command has no place a name could have stood, so writing
		// one would turn `cat` into the coprocess's name.
		{"coproc cat", "coproc cat"},
		// A name that was written is the name.
		{"coproc NM ( : )", "coproc NM ( : )"},
	} {
		f, err := syntax.Parse(c.src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", c.src, err)
		}
		if got := strings.TrimSpace(syntax.PrintFileWith(f, l)); got != c.want {
			t.Errorf("%q printed %q, want %q", c.src, got, c.want)
		}
	}
	// And with no default asked for, nothing is written where nothing was.
	f, err := syntax.Parse("coproc ( : )", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := strings.TrimSpace(syntax.Print(f)); got != "coproc ( : )" {
		t.Errorf("printed %q with no default name asked for, want %q", got, "coproc ( : )")
	}
}

// A command that is nothing but redirections: the blank in front of them is
// the arrangement's to ask for.
func TestAWordlessRedirectionTakesTheBlankItIsAskedFor(t *testing.T) {
	d := syntax.Core()
	f, err := syntax.Parse("> out", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := syntax.Print(f); got != "> out" {
		t.Errorf("printed %q, want %q", got, "> out")
	}
	spaced := syntax.PrintFileWith(f, syntax.Layout{BlankBeforeAWordlessRedirection: true})
	if spaced != " > out" {
		t.Errorf("printed %q with the blank asked for, want %q", spaced, " > out")
	}
	// A command with a word in front of it is unaffected either way, which
	// is what says the field is about the *absence* of one.
	g, err := syntax.Parse("echo hi > out", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, l := range []syntax.Layout{{}, {BlankBeforeAWordlessRedirection: true}} {
		if got := syntax.PrintFileWith(g, l); got != "echo hi > out" {
			t.Errorf("printed %q, want %q", got, "echo hi > out")
		}
	}
}
