// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The three ways `cond ? then : else` can be incomplete, and a `:` standing
// where no conditional opened one.
//
// Each is a parse failure kind of its own rather than a sentence written in
// the parser, because the panel cuts them in two directions at once: bash
// parts a conditional's missing value from an ordinary missing operand and
// writes the same sentence in both positions, and ksh93 does the opposite —
// its `?` position reports the colon it is waiting for and its `:` position
// the ordinary end of input. One field cannot hold both cuts (#1190).

func runConditional(t *testing.T, colonIsAToken bool, d *Diagnostics, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src,
		func(dl *syntax.Dialect) { dl.ArithColonIsAToken = colonIsAToken },
		func(r *Runner) { r.Diagnostics = d })
}

func TestAnIncompleteConditionalIsWordedByPosition(t *testing.T) {
	// A dialect that words all three apart, so each row can be told from the
	// others rather than from a default.
	all := &Diagnostics{
		ArithOperandExpected:  "operand",
		ArithExpressionRanOut: "ran out",
		ArithConditionalThen:  "no then",
		ArithConditionalColon: "no colon",
		ArithConditionalElse:  "no else",
	}
	for _, c := range []struct{ src, want string }{
		{`echo "v=$((1 ?))"`, "no then"},
		{`echo "v=$((1 ? 2))"`, "no colon"},
		{`echo "v=$((1 ? 2 :))"`, "no else"},
		// The ordinary operand failures are untouched, which is what says the
		// three above are the conditional's and not a wording of these.
		{`echo "v=$((1 +))"`, "ran out"},
		{`echo "v=$((+))"`, "ran out"},
	} {
		out, st := runConditional(t, false, all, c.src)
		if !strings.Contains(out, c.want) || st == 0 {
			t.Errorf("%s: got %q (status %d), want %q and a failure", c.src, out, st, c.want)
		}
		if strings.Contains(out, "v=") {
			t.Errorf("%s: got %q, want no value", c.src, out)
		}
	}
}

// And a dialect that words none of them falls through to its ordinary
// end-of-input sentence, which is what two of the four do — so an empty field
// is "as the neighboring failure" rather than "no wording".
func TestAnIncompleteConditionalFallsThroughToTheOrdinarySentence(t *testing.T) {
	plain := &Diagnostics{ArithExpressionRanOut: "ran out", ArithOperandExpected: "operand"}
	for _, src := range []string{`echo "v=$((1 ?))"`, `echo "v=$((1 ? 2 :))"`} {
		if out, _ := runConditional(t, false, plain, src); !strings.Contains(out, "ran out") {
			t.Errorf("%s: got %q, want the ordinary sentence", src, out)
		}
	}
	// The colon is the exception: every shell in the panel words it, so it
	// has no fall-through and the default stands.
	if out, _ := runConditional(t, false, plain, `echo "v=$((1 ? 2))"`); strings.Contains(out, "ran out") {
		t.Errorf("got %q, want the colon's own sentence", out)
	}
}

// A `:` where no `?` opened a conditional. The dialect that reads the byte as
// a math token gets *further* before it complains, which is the whole of the
// difference and the reason two rows are needed: with nothing after the colon
// it runs out of input, and with something after it, it has a complaint only
// a reader that got that far could make.
func TestAStrayColonIsAMathTokenWhereTheDialectSaysSo(t *testing.T) {
	d := &Diagnostics{
		ArithExpressionRanOut:     "ran out",
		ArithOperandExpected:      "operand",
		ArithOperatorExpected:     "leftover",
		ArithColonWithoutQuestion: "no question",
	}
	for _, c := range []struct{ src, token, leftover string }{
		{`echo "v=$((1 :))"`, "ran out", "leftover"},
		{`echo "v=$((1 : 2))"`, "no question", "leftover"},
	} {
		if out, _ := runConditional(t, true, d, c.src); !strings.Contains(out, c.token) {
			t.Errorf("token, %s: got %q, want %q", c.src, out, c.token)
		}
		// Without the flag the colon is text left over — the same *kind*,
		// reached by the other road, so a dialect with a sentence for a
		// stray colon still says it. The reading that changes is how far
		// the reader got, which is the row above: with nothing after the
		// colon the token reading runs out of input and this one does not.
		if out, _ := runConditional(t, false, d, c.src); !strings.Contains(out, "no question") {
			t.Errorf("leftover with a sentence, %s: got %q, want %q", c.src, out, "no question")
		}
		// And a dialect with no sentence for it falls back to the one it
		// gives any leftover text, which is what three of the four say.
		plain := *d
		plain.ArithColonWithoutQuestion = ""
		if out, _ := runConditional(t, false, &plain, c.src); !strings.Contains(out, c.leftover) {
			t.Errorf("leftover, %s: got %q, want %q", c.src, out, c.leftover)
		}
	}
}

// And the conditional still works under the flag, which is the control that
// says the token reading does not eat the colon a `?` is waiting for: the
// then-expression is parsed at the assignment level and recurses through the
// same frame.
func TestAColonThatBelongsToAConditionalIsNotAStrayOne(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "$((1 ? 2 : 3))"`, "2"},
		{`echo "$((0 ? 2 : 3))"`, "3"},
		{`echo "$((1 ? 2 : 3 ? 4 : 5))"`, "2"},
		{`echo "$((0 ? 2 : 0 ? 4 : 5))"`, "5"},
	} {
		out, st := runConditional(t, true, &Diagnostics{}, c.src)
		if strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}
