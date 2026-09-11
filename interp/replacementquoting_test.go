// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a quote in a *replacement* operand comes to, by axis and never by
// shell (#1209).
//
// The third of three readings a quote in a `${ }` operand can take, and the
// only one the panel divides on. A word operand takes the enclosing quoting
// unanimously and a pattern operand's quotes quote, also unanimously —
// quotedbody_test.go holds both — so this one cannot borrow either answer.
func replacementQuoting(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.ReplacementOperandTakesTheEnclosingQuoting = a
		r.Semantics = &s
	}
}

// The two readings, on the characters they actually part on. A quote, a
// backslash and a leading tilde are the three; the rows after them are the
// ones both readings answer alike, which is why they never reach the axis.
func TestAQuotedReplacementOperandAnswersByAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, word, enclosed string }{
		{
			"a parameter between two quotes",
			`s=xay; v=VAL; printf "[%s]" "${s/a/'$v'}"`, `[x$vy]`, `[x'VAL'y]`,
		},
		{
			"and a plain one, where only the quotes differ",
			`s=xay; printf "[%s]" "${s/a/'q'}"`, `[xqy]`, `[x'q'y]`,
		},
		{
			"a command substitution between them still runs",
			"s=xay; printf \"[%s]\" \"${s/a/'`echo B`'}\"", "[x`echo B`y]", `[x'B'y]`,
		},
		{
			"an empty pair is nothing, or two characters",
			`s=xay; printf "[%s]" "${s/a/''}"`, `[xy]`, `[x''y]`,
		},
		{
			"a backslash escapes, or stands before what it wrote",
			`s=xay; printf "[%s]" "${s/a/\q}"`, `[xqy]`, `[x\qy]`,
		},
		{
			"a leading tilde expands, or does not",
			`s=xay; HOME=/HH; printf "[%s]" "${s/a/~}"`, `[x/HHy]`, `[x~y]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := run(t, tc.src, replacementQuoting(No)); out != tc.word || st != 0 {
				t.Errorf("no: %s = %q (status %d), want %q at 0", tc.src, out, st, tc.word)
			}
			if out, st := run(t, tc.src, replacementQuoting(Yes)); out != tc.enclosed || st != 0 {
				t.Errorf("yes: %s = %q (status %d), want %q at 0", tc.src, out, st, tc.enclosed)
			}
		})
	}
}

// Asked only at the disagreement, and this is the test that says so: with no
// answer at all, every one of these still produces its value. An axis reached
// on the common path would refuse them by name.
func TestAReplacementOperandThatCannotDifferNeverAsks(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an ordinary replacement", `s=xay; printf "[%s]" "${s/a/b}"`, `[xby]`},
		{"a parameter in one", `s=xay; v=VAL; printf "[%s]" "${s/a/$v}"`, `[xVALy]`},
		{"a double quote, which both readings remove", `s=xay; printf "[%s]" "${s/a/"q"}"`, `[xqy]`},
		{"a tilde that is not at the front", `s=xay; printf "[%s]" "${s/a/p~q}"`, `[xp~qy]`},
		{"a glob character", `s=xay; printf "[%s]" "${s/a/*}"`, `[x*y]`},
		// Unquoted is the row that says the disagreement belongs to the
		// enclosing context rather than to the operator.
		{"unquoted, whatever it holds", `s=xay; v=VAL; printf "[%s]" ${s/a/'$v'}`, `[x$vy]`},
		// And the pattern half never takes it, quoted or not.
		{"a quote in the pattern half", `s=xay; printf "[%s]" "${s/'a'/Z}"`, `[xZy]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, replacementQuoting(Unspecified))
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// No answer is refused by name where it *is* asked. Both readings produce a
// plausible string at status 0, so guessing would be a silent wrong answer —
// which is the shape this refusal exists for.
//
// The command that depended on it does not run and the list carries on, which
// is what every other expansion-level axis does — `${a[*]#a}` and `${x:h}`
// answer this way too, and a refusal shaped differently from its neighbours
// would be a second convention rather than a stricter one.
func TestAQuotedReplacementOperandUnansweredIsRefused(t *testing.T) {
	src := `s=xay; printf "[%s]" "${s/a/'q'}"; echo after`
	out, _ := run(t, src, replacementQuoting(Unspecified))
	if !strings.Contains(out, "a quote in a quoted replacement operand") ||
		!strings.Contains(out, "no dialect was chosen") {
		t.Errorf("got %q, want the axis named in the refusal", out)
	}
	if strings.Contains(out, "[x") {
		t.Errorf("got %q, want no value from the expansion", out)
	}
}

// The element operator takes the same operand and must reach the same answer:
// two spellings of one rule is how a fix comes undone in the branch nobody
// looks at.
func TestTheElementReplacementTakesTheSameAxis(t *testing.T) {
	src := `a=(xay); v=VAL; printf "[%s]" "${a[@]/a/'$v'}"`
	if out, _ := run(t, src, replacementQuoting(No)); out != `[x$vy]` {
		t.Errorf("no: %s = %q, want %q", src, out, `[x$vy]`)
	}
	if out, _ := run(t, src, replacementQuoting(Yes)); out != `[x'VAL'y]` {
		t.Errorf("yes: %s = %q, want %q", src, out, `[x'VAL'y]`)
	}
}
