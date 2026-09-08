// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What is between the parentheses of `<( )` and `>( )` is a program, so a `#`
// in it opens a comment — and the content of a comment cannot affect the
// parse.
//
// Measured across the panel, not reasoned: bash 5.3, that build called `sh`,
// ksh93 and zsh all read the comment, which is every member that has the
// construct. dash has none, so it records the grammar's absence. bash 3.2 is
// the one column that disagrees, and it disagrees by having exactly the bug
// this fixes — `bad substitution: no closing )` — so this is one grammar
// rather than a `Dialect` flag.
//
// The apostrophe is the point of the shapes below. Two failures compounded
// here: the comment was not a comment, so its text was parsed as code, and
// then the apostrophe in it opened a single quote that ran to the end of the
// input. A real script refused 214 lines from its comment is what found it
// (#1397).
func TestACommentInAProcessSubstitutionBodyIsAComment(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		kind      syntax.SpanKind
		inner     string
	}{
		{
			"an apostrophe in a comment line",
			"cat < <(\n\t# it's a comment\n\techo hi\n)\n",
			syntax.ProcSubstIn, "\n\t# it's a comment\n\techo hi\n",
		},
		{
			"and writing to one",
			"echo x > >(\n\t# it's a comment\n\tcat\n)\n",
			syntax.ProcSubstOut, "\n\t# it's a comment\n\tcat\n",
		},
		{
			// A comment opens where a word could begin, and after a
			// separator with no blank in between is such a place. A rule
			// written as "preceded by whitespace" passes the rows above and
			// fails these three.
			"after a semicolon with no blank",
			"cat < <(echo hi;# it's tight\n)\n",
			syntax.ProcSubstIn, "echo hi;# it's tight\n",
		},
		{
			"after a pipe with no blank",
			"cat < <(echo hi |# it's tight\ncat\n)\n",
			syntax.ProcSubstIn, "echo hi |# it's tight\ncat\n",
		},
		{
			"against the opening parenthesis",
			"cat < <(#it's tight\necho hi\n)\n",
			syntax.ProcSubstIn, "#it's tight\necho hi\n",
		},
		{
			// The comment ends at the newline, so the `)` on the same line
			// as the comment's text is inside it and the one on the next
			// line is the closer.
			"a comment on the line before the closer",
			"cat < <(echo hi # it's inline\n)\n",
			syntax.ProcSubstIn, "echo hi # it's inline\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			span, ok := findSpan(f, tc.kind)
			if !ok {
				t.Fatalf("no %v span in %q", tc.kind, tc.src)
			}
			// The whole body, exactly. A Contains check would pass on a
			// span that had run off the end of the construct and taken the
			// rest of the file with it, which is the failure being fixed.
			if span.Value != tc.inner {
				t.Errorf("inner = %q, want %q", span.Value, tc.inner)
			}
		})
	}
}

// The other side of the rule, and the side that makes the fix falsifiable
// rather than merely permissive: a comment runs to the newline, so a `)`
// written inside one closes nothing.
//
// It is what a scanner that gained the comment rule in only one of its two
// routes gets wrong, and it gets it wrong in the *accepting* direction, which
// is the dangerous one — the parser refuses the body, the paren-counting
// fallback then finds that `)` and calls the substitution closed. Every panel
// member that has the construct refuses these.
func TestACommentDoesNotEndAtAClosingParenthesis(t *testing.T) {
	for _, src := range []string{
		"cat < <(echo hi # cmt )\necho after\n",
		"echo x > >(cat # cmt )\necho after\n",
		"cat < <(echo hi # it's a comment )\necho after\n",
	} {
		if _, err := syntax.Parse(src, syntax.Core()); err == nil {
			t.Errorf("%q parsed, want a refusal: the `)` is inside the comment", src)
		}
	}
}

// And the counter-case that keeps the rule from being a blanket one: mid-word
// a `#` is an ordinary character. `cat < <(echo a#b)` prints `a#b` in all
// five shells that have the construct, bash 3.2 included.
//
// This is the row a fix that skipped to the newline on every `#` would fail:
// that fix eats `#b)` and the substitution never closes.
func TestAHashMidWordInAProcessSubstitutionBodyIsNotAComment(t *testing.T) {
	for _, tc := range []struct{ src, inner string }{
		{"cat < <(echo a#b)\n", "echo a#b"},
		{"cat < <(echo a#b\n)\n", "echo a#b\n"},
		{"echo x > >(grep a#b)\n", "grep a#b"},
	} {
		f, err := syntax.Parse(tc.src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		kind := syntax.ProcSubstIn
		if strings.Contains(tc.src, "> >(") {
			kind = syntax.ProcSubstOut
		}
		span, ok := findSpan(f, kind)
		if !ok {
			t.Fatalf("no %v span in %q", kind, tc.src)
		}
		if span.Value != tc.inner {
			t.Errorf("%q: inner = %q, want %q", tc.src, span.Value, tc.inner)
		}
	}
}

// The `#` in arithmetic is not a comment — it is the base separator — so the
// comment rule the two program-holding kinds get must not reach the kind whose
// parentheses hold an expression.
//
// Measured: `$(( 16#ff ))` is 255 in bash 5.3, bash 3.2, ksh93 and zsh, and
// all four call `$(( 1 # c ))` an arithmetic syntax error rather than reading
// a comment. The gate is `holdsCommands`, and this is what it is for.
func TestAHashInArithmeticIsNotAComment(t *testing.T) {
	f, err := syntax.Parse("echo $(( 16#ff ))\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	span, ok := findSpan(f, syntax.ArithSubst)
	if !ok {
		t.Fatal("no ArithSubst span")
	}
	if span.Value != " 16#ff " {
		t.Errorf("inner = %q, want %q — a comment rule reaching arithmetic eats the base", span.Value, " 16#ff ")
	}
}

// The body of `${ cmd;}` holds a program too, and the brace scanner had the
// same hole — the sibling found by looking for one before writing the fix
// rather than after.
//
// It cannot take a blanket comment rule: `#` is the strip operator in
// `${x#a}` and the length operator in `${#x}`. So the rule is gated on the
// form, which is why both halves are asserted here.
func TestACommentInACurrentShellSubstitutionBodyIsAComment(t *testing.T) {
	d := syntax.Core()
	d.CurrentShellSubstitution = true

	t.Run("a comment is a comment", func(t *testing.T) {
		src := "echo ${\n\t# it's a comment\n\techo hi\n}\n"
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		span, ok := findSpan(f, syntax.CommandSubst)
		if !ok {
			t.Fatalf("no CommandSubst span in %q", src)
		}
		if want := "\n\t# it's a comment\n\techo hi\n"; span.Value != want {
			t.Errorf("inner = %q, want %q", span.Value, want)
		}
	})

	t.Run("a comment does not end at the closing brace", func(t *testing.T) {
		// bash 5.3 says `unexpected EOF while looking for matching }` and
		// ksh93 says `{ unmatched`; both refuse.
		if _, err := syntax.Parse("a=${ echo hi # cmt }\necho after\n", d); err == nil {
			t.Error("parsed, want a refusal: the `}` is inside the comment")
		}
	})

	t.Run("mid-word is not a comment", func(t *testing.T) {
		src := "echo ${ echo x#y; }\n"
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		span, ok := findSpan(f, syntax.CommandSubst)
		if !ok {
			t.Fatalf("no CommandSubst span in %q", src)
		}
		if want := " echo x#y; "; span.Value != want {
			t.Errorf("inner = %q, want %q", span.Value, want)
		}
	})

	// The two expansions the same character spells, which a blanket rule
	// inside braces would break. Unanimous across all six.
	t.Run("the strip and length operators survive", func(t *testing.T) {
		for _, tc := range []struct{ src, inner string }{
			{"echo ${x#a}\n", "x#a"},
			{"echo ${x##a}\n", "x##a"},
			{"echo ${#x}\n", "#x"},
		} {
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			span, ok := findSpan(f, syntax.ParamExp)
			if !ok {
				t.Fatalf("no ParamExp span in %q", tc.src)
			}
			if span.Value != tc.inner {
				t.Errorf("%q: inner = %q, want %q", tc.src, span.Value, tc.inner)
			}
		}
	})
}
