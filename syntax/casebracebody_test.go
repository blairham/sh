// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

func caseBraces() syntax.Dialect {
	d := syntax.Core()
	d.CaseBraceBody = syntax.CaseBraceBodyMixesWithTheKeyword
	// Both brace rules travel with it in the only preset that has any of
	// them, and each is load-bearing for a row below: without the first a
	// `}` at the end of an arm's last word is ordinary text, and without the
	// second `case x {x)` lexes `{x` as one word.
	d.CloseBraceAlwaysReserved = true
	d.OpenBraceNeedsNoBlank = true
	return d
}

// A `case` may be written with braces in place of `in` … `esac`, and the two
// halves are independent: either opener composes with either closer.
func TestACaseMayBeWrittenWithBraces(t *testing.T) {
	for _, src := range []string{
		"case x { x) echo hit;; }\n",
		"case x { x) echo hit;; *) echo no;; }\n",
		// The last arm needs no terminator before the closer, brace or not.
		"case x { x) echo hit }\n",
		// And no arm at all.
		"case x { }\n",
		// Brace open, keyword close.
		"case x { x) echo hit;; esac\n",
		// Keyword open, brace close.
		"case x in x) echo hit;; }\n",
		// The `{` needs no blank after it, as in command position.
		"case x {x) echo hit;; }\n",
		// A pattern may still carry its own leading paren.
		"case x { (x) echo hit;; }\n",
		"case x {\nx) echo hit;;\n}\n",
	} {
		if _, err := syntax.Parse(src, caseBraces()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
		if _, err := syntax.Parse(src, syntax.Core()); err == nil {
			// Two of the rows are the keyword spelling with a brace closer,
			// which the core also refuses, so no row is excused here.
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// The tree is a `case`'s either way. Nothing in it records which spelling was
// written, which is what a formatter's Style answers instead.
func TestABraceSpelledCaseIsACaseClause(t *testing.T) {
	brace := onlyCommand(t, "case x { x) echo hit;; }\n", caseBraces())
	keyword := onlyCommand(t, "case x in x) echo hit;; esac\n", caseBraces())
	b, ok := brace.(*syntax.CaseClause)
	if !ok {
		t.Fatalf("brace spelling parsed to %T, want *syntax.CaseClause", brace)
	}
	k, ok := keyword.(*syntax.CaseClause)
	if !ok {
		t.Fatalf("keyword spelling parsed to %T, want *syntax.CaseClause", keyword)
	}
	if len(b.Items) != len(k.Items) || len(b.Items) != 1 {
		t.Fatalf("arms: brace %d, keyword %d, want 1 each", len(b.Items), len(k.Items))
	}
}

// Whether the two words are paired is a second answer, and the spelling is
// what carries it: one value takes all four combinations and the other only
// the two matched pairs.
func TestWhetherTheCaseBracesPairWithTheirOpener(t *testing.T) {
	paired := syntax.Core()
	paired.CaseBraceBody = syntax.CaseBraceBodyPairsWithItsOpener
	for _, tc := range []struct {
		src          string
		mixes, pairs bool
	}{
		{"case x { x) echo hit;; }\n", true, true},
		{"case x in x) echo hit;; esac\n", true, true},
		{"case x { x) echo hit;; esac\n", true, false},
		{"case x in x) echo hit;; }\n", true, false},
		// A `case` with no arm at all closes on the `}` under both, which is
		// what says the terminator reading below is about the word `esac`
		// and never about the brace.
		{"case x { }\n", true, true},
	} {
		for name, col := range map[string]struct {
			d    syntax.Dialect
			want bool
		}{
			"mixes": {caseBraces(), tc.mixes},
			"pairs": {paired, tc.pairs},
		} {
			_, err := syntax.Parse(tc.src, col.d)
			if got := err == nil; got != col.want {
				t.Errorf("%s: %q parsed = %v, want %v (%v)", name, tc.src, got, col.want, err)
			}
		}
	}
}

// The terminator reading travels with the header's *position* rather than
// with the word `in`, so a dialect that has both flags reads `esac` after the
// `{` as a pattern too — and still closes on a `}`.
func TestTheTerminatorReadingFollowsEitherOpener(t *testing.T) {
	d := syntax.Core()
	d.CaseBraceBody = syntax.CaseBraceBodyPairsWithItsOpener
	d.CaseTerminatorIsAPatternAfterTheHeader = true
	if _, err := syntax.Parse("case esac { esac) echo hit;; }\n", d); err != nil {
		t.Errorf("`case esac { esac) … }`: %v", err)
	}
	if _, err := syntax.Parse("case x { esac\n", d); err == nil {
		t.Error("`case x { esac` parsed; the `esac` is a pattern here")
	}
	if _, err := syntax.Parse("case x { }\n", d); err != nil {
		t.Errorf("`case x { }`: %v — the `}` is never a pattern", err)
	}
}

// Without that flag, the opener does not carry the reading with it: `esac`
// stays reserved after the `{`.
func TestEsacIsStillReservedAfterTheBrace(t *testing.T) {
	if _, err := syntax.Parse("case esac { esac) echo hit;; }\n", caseBraces()); err == nil {
		t.Error("`case esac { esac) … }` parsed; the `esac` closes the case here")
	}
	// A leading paren takes the reservation away, as everywhere else.
	if _, err := syntax.Parse("case esac { (esac) echo hit;; }\n", caseBraces()); err != nil {
		t.Errorf("a parenthesized `esac` pattern: %v", err)
	}
}
