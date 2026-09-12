// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `case $x in (|https|git) …` — a pattern list one of whose alternatives is
// written as nothing, so the arm also matches the empty subject. The grammar
// half; what it then matches is interp/emptycasepattern_test.go.

// emptyAlt is the core with the flag on, and nothing else: the empty
// alternative is independent of pattern alternation inside a word, which the
// same shell also has.
func emptyAlt() syntax.Dialect {
	d := syntax.Core()
	d.CasePatternMayBeEmpty = true
	return d
}

// casePatterns returns the arm's patterns as written, with an alternative that
// was written as nothing coming back as the empty string.
func casePatterns(t *testing.T, src string, d syntax.Dialect) []string {
	t.Helper()
	c, ok := onlyCommand(t, src, d).(*syntax.CaseClause)
	if !ok {
		t.Fatalf("parse %q: not a case", src)
	}
	if len(c.Items) == 0 {
		t.Fatalf("parse %q: no arms", src)
	}
	out := make([]string, 0, len(c.Items[0].Patterns))
	for _, p := range c.Items[0].Patterns {
		out = append(out, p.Literal())
	}
	return out
}

// Every place a separator can put an empty alternative, and each one is a list
// of the length the separators say rather than of the words that were written.
// Measured 2026-09-06 on zsh 5.9.2 with `-n` over a script file: all seven
// parse there.
func TestASeparatorMayHaveNoPatternOnEitherSideOfIt(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"leading", "case a in (|x|y) echo m;; esac", []string{"", "x", "y"}},
		{"leading without the arm's paren", "case a in |x|y) echo m;; esac", []string{"", "x", "y"}},
		{"middle", "case a in (x||y) echo m;; esac", []string{"x", "", "y"}},
		{"trailing", "case a in (x|y|) echo m;; esac", []string{"x", "y", ""}},
		{"nothing but a separator", "case a in (|) echo m;; esac", []string{"", ""}},
		{"nothing but two", "case a in (||) echo m;; esac", []string{"", "", ""}},
		{"spaces around the separators", "case a in ( | x | y ) echo m;; esac", []string{"", "x", "y"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := casePatterns(t, tc.src, emptyAlt())
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("patterns = %q, want %q", got, tc.want)
			}
			// The whole of what the flag does: without it the same line is
			// a syntax error rather than a shorter list.
			mustFailHere(t, tc.src, syntax.Core(), "without the flag")
		})
	}
}

// The list may also be written as nothing at all, where the arm's parentheses
// are there to hold it — so the emptiness is the *list's* rather than
// something read off a separator, which is what this file used to say.
//
// Measured 2026-09-12 on zsh 5.9.2 over a script file: `case "" in ( ) echo
// em;; (*) echo star;; esac` prints `em`, and the same line with a subject of
// one blank prints `star`. So the pattern is the empty string and not the
// blank between the parentheses, which that dialect trims either side of a
// list.
func TestTheWholeListMayBeWrittenAsNothingInsideTheArmsParens(t *testing.T) {
	for _, src := range []string{
		"case a in ( ) echo m;; esac",
		"case a in (  ) echo m;; esac",
	} {
		got := casePatterns(t, src, emptyAlt())
		if want := ""; strings.Join(got, ",") != want {
			t.Errorf("%q: patterns = %q, want one empty alternative", src, got)
		}
		mustFailHere(t, src, syntax.Core(), "without the flag")
	}
}

// And nowhere else: written without the arm's parentheses there is nothing to
// hold an empty list, and the shell that accepts every line above refuses it.
func TestAPatternListMayNotBeEmptyWithoutTheArmsParens(t *testing.T) {
	mustFailHere(t, "case a in ) echo m;; esac", emptyAlt(), "no parens to hold an empty list")
}

// `()` is refused there, and not for the emptiness: the pair is one token to
// the dialect that reads it as one, so the `(` never opens an arm at all and
// the refusal names both characters. `( )` — the same two with a blank
// between them — is the arm above and parses.
//
// Measured 2026-09-12 on zsh 5.9.2: `case a in () echo m;; esac` is “parse
// error near `()' “ and `case a in (a) echo m;; () esac` is the same, where
// this parser named the `)' alone (#1111).
func TestEmptyParensDoNotOpenAnArm(t *testing.T) {
	d := emptyAlt()
	d.EmptyParensAreOneToken = true
	for _, src := range []string{
		"case a in () echo m;; esac",
		"case a in (a) echo m;; () esac",
	} {
		_, err := syntax.Parse(src, d)
		if err == nil {
			t.Fatalf("parse %q: no error, want the pair refused", src)
		}
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("parse %q: err = %v, want a *syntax.Error", src, err)
		}
		if se.Token != "()" {
			t.Errorf("parse %q: blamed %q, want %q — the pair, which is one token here", src, se.Token, "()")
		}
	}
	// Without the pair being one token the `(` opens an arm as it always
	// did, and the empty list above is what stands inside it.
	if got := casePatterns(t, "case a in () echo m;; esac", emptyAlt()); strings.Join(got, ",") != "" {
		t.Errorf("without the flag: patterns = %q, want one empty alternative", got)
	}
}

// `||` arrives as one token, because the lexer reads the operator before
// anything has said this is a pattern list. It has to be taken apart into two
// separators with a pattern of nothing between them — and where the flag is
// off it stays the operator it was lexed as, which is the token the four
// shells without this blame.
func TestTheDoubledSeparatorIsTwoOfThemAndNotAnOperator(t *testing.T) {
	got := casePatterns(t, "case a in (x||y) echo m;; esac", emptyAlt())
	if want := "x,,y"; strings.Join(got, ",") != want {
		t.Errorf("patterns = %q, want %q", got, want)
	}
	_, err := syntax.Parse("case a in (x||y) echo m;; esac", syntax.Core())
	if err == nil {
		t.Fatal("parsed without the flag, want a syntax error")
	}
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want a *syntax.Error", err)
	}
	if se.Token != "||" {
		t.Errorf("blamed %q, want %q — the whole operator, as the four shells without this do", se.Token, "||")
	}
}

// The printed form reads back as the same list. An alternative written as
// nothing prints as nothing, so the separators are the only thing carrying it
// and a printer that dropped an empty word would round-trip to a shorter list.
func TestAnEmptyAlternativeSurvivesBeingPrinted(t *testing.T) {
	const src = "case a in (|x|y) echo m;; esac"
	f, err := syntax.Parse(src, emptyAlt())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	printed := syntax.Print(f)
	got := casePatterns(t, printed, emptyAlt())
	if want := ",x,y"; strings.Join(got, ",") != want {
		t.Errorf("printed as %q, which reads back with patterns %q, want %q", printed, got, want)
	}
}

// An empty alternative is unquoted and has no spans, which is what makes it
// expand to the empty string rather than to nothing at all — and it is located
// at the separator that says it is there, so a diagnostic about the arm has a
// position to point at.
func TestAnEmptyAlternativeIsLocatedAtItsSeparator(t *testing.T) {
	const src = "case a in (|x) echo m;; esac"
	c, ok := onlyCommand(t, src, emptyAlt()).(*syntax.CaseClause)
	if !ok {
		t.Fatalf("parse %q: not a case", src)
	}
	first := c.Items[0].Patterns[0]
	if first.IsQuoted() {
		t.Error("the empty alternative reports itself quoted, so it would match a literal rather than nothing")
	}
	if n := len(first.Spans); n != 0 {
		t.Errorf("spans = %d, want 0", n)
	}
	// The `|` is the eleventh byte of the line.
	if got, want := int(first.Pos().Offset), strings.IndexByte(src, '|'); got != want {
		t.Errorf("located at offset %d, want %d — the separator that says it is there", got, want)
	}
	if first.End() != first.Pos() {
		t.Errorf("spans %v..%v, want an empty extent", first.Pos(), first.End())
	}
}

// The two ways a pattern position may hold something other than a word are
// asked in a fixed order, and no preset sets both — dash has the operator slot
// and zsh has the empty alternative. So the order is unobservable in every
// shell that exists, and this is the test that makes it observable at all: a
// dialect with both must read `|` as the separator it is rather than as an
// operator standing where a pattern belongs, because the other order consumes
// the `|` and then finds a word where the `)` should be.
func TestTheEmptyAlternativeIsAskedBeforeTheOperatorSlot(t *testing.T) {
	d := emptyAlt()
	d.CasePatternAcceptsOperator = true
	got := casePatterns(t, "case a in (|x|y) echo m;; esac", d)
	if want := ",x,y"; strings.Join(got, ",") != want {
		t.Errorf("patterns = %q, want %q", got, want)
	}
	// The slot is still there for what it is for, and it still contributes
	// no pattern: an operator is taken and the arm keeps the rest.
	if got, want := casePatterns(t, "case a in (x| ; |y) echo m;; esac", d), "x,y"; strings.Join(got, ",") != want {
		t.Errorf("patterns = %q, want %q", got, want)
	}
}
