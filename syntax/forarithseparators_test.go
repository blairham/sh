// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// forArithDialects is the C-style loop with the extra-separator flag both
// ways, and nothing else set, so what a case proves is about the count.
func forArithDialects() (folds, refuses syntax.Dialect) {
	folds, refuses = syntax.POSIX(), syntax.POSIX()
	folds.CStyleFor, folds.ArithCommand, folds.ForArithExtraSeparators = true, true, true
	refuses.CStyleFor, refuses.ArithCommand = true, true
	return folds, refuses
}

// A C-style `for` header needs exactly two separators, and *fewer* is refused
// under either setting of the flag: the flag is about the surplus and not
// about the shortfall.
//
// The severity is in what accepting one would mean rather than in the refusal.
// A header with no separators has no condition, an absent condition is true,
// and so the loop is endless and its body is unbounded output (#2225).
func TestAForHeaderNeedsTwoSeparators(t *testing.T) {
	folds, refuses := forArithDialects()
	for _, header := range []string{"(())", "(( ))", "((;))", "((1;2))", "((i=0))", "((\ni=0\n))"} {
		src := "for " + header + "; do echo x; break; done\n"
		for name, d := range map[string]syntax.Dialect{"folds": folds, "refuses": refuses} {
			_, err := syntax.Parse(src, d)
			var se *syntax.Error
			if !errors.As(err, &se) || se.Kind != syntax.ErrForArithHeader {
				t.Errorf("%s %q: got %v, want ErrForArithHeader", name, src, err)
				continue
			}
			// The header as written travels with the error, parentheses and
			// newlines kept, because one dialect echoes it back whole.
			if se.Token != header {
				t.Errorf("%s %q: Token %q, want %q", name, src, se.Token, header)
			}
			// And the line the header *opens* on, which for the multi-line
			// case is not the line the count ran out on.
			if se.Pos.Line != 1 {
				t.Errorf("%s %q: line %d, want 1", name, src, se.Pos.Line)
			}
		}
	}
}

// LastToken is the header's last section trimmed, which is what one dialect
// names and is empty where that section holds nothing. It is the section as
// the header wrote it rather than as the three-part split pads it: a header of
// one section has that section as its last.
func TestARefusedForHeaderCarriesItsLastSection(t *testing.T) {
	_, refuses := forArithDialects()
	for header, last := range map[string]string{
		"((i=0))":   "i=0",
		"((1;2))":   "2",
		"((;2))":    "2",
		"(( x=1 ))": "x=1",
		"(())":      "",
		"((;))":     "",
		"((1;))":    "",
		"(( x ; ))": "",
	} {
		_, err := syntax.Parse("for "+header+"; do echo x; break; done\n", refuses)
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("%q: got %v, want a refusal", header, err)
		}
		if se.LastToken != last {
			t.Errorf("%q: LastToken %q, want %q", header, se.LastToken, last)
		}
	}
}

// More than two separators is the flag's question, and it is a separate kind
// because the dialect that refuses it says something else about it.
func TestExtraSeparatorsAreFoldedOrRefused(t *testing.T) {
	folds, refuses := forArithDialects()
	for _, header := range []string{"((;;;))", "((;;;;))", "((1;2;3;4))", "(( ; ; ; ))"} {
		src := "for " + header + "; do echo x; break; done\n"
		if _, err := syntax.Parse(src, folds); err != nil {
			t.Errorf("%q should parse with the flag on: %v", src, err)
		}
		_, err := syntax.Parse(src, refuses)
		var se *syntax.Error
		if !errors.As(err, &se) || se.Kind != syntax.ErrForArithSeparator {
			t.Errorf("%q: got %v, want ErrForArithSeparator", src, err)
			continue
		}
		if se.Token != header {
			t.Errorf("%q: Token %q, want %q", src, se.Token, header)
		}
	}
}

// Exactly two parses under both, in every shape a section may be left out —
// which is the boundary the count has to keep, since `for ((;;))` is how the
// endless loop is spelled and holds no expression at all.
func TestTwoSeparatorsParseWhateverTheSectionsHold(t *testing.T) {
	folds, refuses := forArithDialects()
	for _, src := range []string{
		"for ((;;)); do echo x; break; done\n",
		"for (( ; ; )); do echo x; break; done\n",
		"for ((i=0;;)); do echo x; break; done\n",
		"for ((;i<1;)); do echo x; break; done\n",
		"for ((;;i=i+1)); do echo x; break; done\n",
		"for ((i=0;i<2;i=i+1)); do echo $i; done\n",
		"for ((\ni=0;\ni<2;\ni=i+1\n)); do echo $i; done\n",
		"for ((;;)) do echo x; break; done\n",
	} {
		for name, d := range map[string]syntax.Dialect{"folds": folds, "refuses": refuses} {
			if _, err := syntax.Parse(src, d); err != nil {
				t.Errorf("%s %q should parse: %v", name, src, err)
			}
		}
	}
}

// The folded header keeps its three sections, with everything past the second
// separator in the third — text the arithmetic reader will refuse if the loop
// ever evaluates it, which is what makes the acceptance bounded rather than a
// second way to lose the count.
func TestTheFoldedHeaderKeepsTheSurplusInItsLastSection(t *testing.T) {
	folds, _ := forArithDialects()
	f, err := syntax.Parse("for ((i=0;i<2;i=i+1;i=9)); do echo x; done\n", folds)
	if err != nil {
		t.Fatalf("should parse: %v", err)
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok {
		t.Fatalf("got %T, want *syntax.Pipeline", f.Stmts[0].Expr)
	}
	c, ok := pipe.Cmds[0].(*syntax.ForArithClause)
	if !ok {
		t.Fatalf("got %T, want *syntax.ForArithClause", pipe.Cmds[0])
	}
	if c.InitText != "i=0" || c.CondText != "i<2" || c.PostText != "i=i+1;i=9" {
		t.Errorf("sections %q %q %q, want \"i=0\" \"i<2\" \"i=i+1;i=9\"",
			c.InitText, c.CondText, c.PostText)
	}
}
