// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The two arrangements a listing needs to write the inside of a `$( … )` —
// [syntax.Layout.KeywordBodiesTakeTheirOwnLines] and
// [syntax.Layout.CommandSubstitutionIsReprinted], and #3801, where a span's
// characters went back out unchanged because nothing here can re-read them.
//
// Named for the fields; which engine asks for them, and what its own bytes
// are, is in dialect/bash.

// keywordBodies is the third arrangement: a list keeps the source's own line
// breaks, and what a reserved word closes is laid out.
func keywordBodies() syntax.Layout {
	return syntax.Layout{
		Indent: "    ", Nested: true,
		Separator: ";", KeywordTerminator: ";",
		BraceOpenSuffix:                " ",
		CaseHeaderSuffix:               " ",
		KeywordBodiesTakeTheirOwnLines: true,
	}
}

// TestKeywordBodiesTakeTheirOwnLinesIsNeitherOfTheOtherTwo is the field's
// whole claim, and each row is one half of it.
//
// A list is not laid out — that is what [syntax.Layout.Lines] would do and it
// is off here — and a construct is, which is what leaving Lines off would not
// do. The brace group is the control that says the split is the **closing
// token** rather than the nesting: a body the brackets close stays where it
// was written under the same arrangement that spreads an `if`.
func TestKeywordBodiesTakeTheirOwnLinesIsNeitherOfTheOtherTwo(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a list on one line", "a; b\n", "a; b"},
		{"a list over two", "a\nb\n", "a\nb"},
		{"a branch", "if x; then y; fi\n", "if x; then\n    y;\nfi"},
		{"a loop", "while x; do y; done\n", "while x; do\n    y;\ndone"},
		{"a brace group", "{ a; b; }\n", "{ a; b; }"},
		{"a subshell", "(a; b)\n", "( a; b )"},
		{"a construct inside a list", "a; if x; then y; fi; b\n", "a; if x; then\n    y;\nfi; b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := syntax.PrintFileWith(f, keywordBodies()); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// reprintWith is the hook a caller supplies: parse with its own grammar, print
// with its own arrangement, hand the text back.
func reprintWith(l syntax.Layout) func(string) (string, bool) {
	reprint := func(body string) (string, bool) {
		f, err := syntax.Parse(body, syntax.Core())
		if err != nil {
			return "", false
		}
		return syntax.PrintFileWith(f, l), true
	}
	l.CommandSubstitutionIsReprinted = reprint
	return reprint
}

// TestACommandSubstitutionsBodyGoesThroughTheHook is the second field: the
// span holds text, and an arrangement with a reader for it writes what the
// reader hands back.
//
// The nil row above it is the one that says the hook is what moves this — the
// same source, the same layout, and the characters back out unchanged.
func TestACommandSubstitutionsBodyGoesThroughTheHook(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, plain, hooked string }{
		{"spacing inside the body", "echo $(a >/dev/null;b)\n", "echo $(a >/dev/null;b)", "echo $(a > /dev/null; b)"},
		{"a construct inside it", "echo $(if x; then y; fi)\n", "echo $(if x; then y; fi)", "echo $(if x; then\n    y;\nfi)"},
		{"one inside another", "echo $(echo $(a;b))\n", "echo $(echo $(a;b))", "echo $(echo $(a; b))"},
		{"an empty body", "echo $( )\n", "echo $( )", "echo $()"},
		// The control the whole field rests on: a backquoted substitution is
		// the same node with a flag set, and the hook must not reach it.
		{"a backquoted body", "echo `a >/dev/null;b`\n", "echo `a >/dev/null;b`", "echo `a >/dev/null;b`"},
		// And the one that keeps the output a program. The body comes back
		// beginning with `(`, and `$((` opens arithmetic — so the blank that
		// the arrangement writes is decided by what is about to be written
		// and not by the characters the span holds.
		{"a subshell body", "echo $( (a;b) )\n", "echo $( (a;b) )", "echo $( ( a; b ))"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := syntax.PrintFileWith(f, keywordBodies()); got != c.plain {
				t.Errorf("with no hook: got %q, want %q", got, c.plain)
			}
			l := keywordBodies()
			l.CommandSubstitutionIsReprinted = reprintWith(keywordBodies())
			if got := syntax.PrintFileWith(f, l); got != c.hooked {
				t.Errorf("hooked: got %q, want %q", got, c.hooked)
			}
		})
	}
}

// TestABodyTheHookRefusesIsWrittenBack is the best-effort half, and it is a
// property rather than a nicety: a substitution's body is not read until the
// substitution runs, so a tree can legitimately hold one that will not parse.
// An arrangement that dropped it, or refused to print at all, would lose a
// program it could otherwise run.
func TestABodyTheHookRefusesIsWrittenBack(t *testing.T) {
	t.Parallel()
	const src = "echo $(for in)\n"
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	asked := 0
	l := keywordBodies()
	l.CommandSubstitutionIsReprinted = func(body string) (string, bool) {
		asked++
		return "", false
	}
	if got := syntax.PrintFileWith(f, l); got != "echo $(for in)" {
		t.Errorf("got %q, want the body written back as it stands", got)
	}
	if asked != 1 {
		t.Errorf("the hook was asked %d times, want 1", asked)
	}
}

// TestTheHookIsNotAskedWhereNothingAsksForIt is the third control: an
// arrangement that leaves the field nil is every other caller of this printer,
// a formatter included, and a formatter that re-read a body would be editing
// the program it was asked to lay out.
func TestTheHookIsNotAskedWhereNothingAsksForIt(t *testing.T) {
	t.Parallel()
	const src = "echo $(a;b) `c;d` $((1+1)) ${x-a  b}\n"
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := syntax.PrintFileWith(f, syntax.Layout{})
	if !strings.Contains(got, "$(a;b)") || !strings.Contains(got, "`c;d`") ||
		!strings.Contains(got, "$((1+1))") || !strings.Contains(got, "${x-a  b}") {
		t.Errorf("got %q, want every span written back as it stands", got)
	}
}
