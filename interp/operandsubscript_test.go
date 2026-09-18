// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func runOperandQuoting(t *testing.T, q OperandSubscriptQuotingPolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		// The *store* side, so that a key holding a `]` can be written at
		// all: the operand's scan is what is under test and it can only be
		// asked about an element that exists.
		d.SubscriptSpansSeparators = true
		d.SubscriptQuoteProtectsTheClosingBracket = syntax.SubscriptBackslashQuotes |
			syntax.SubscriptSingleQuotes | syntax.SubscriptDoubleQuotes
	}, func(r *Runner) {
		sem := *r.Semantics
		// The key's own reading, so that `a['x]y']=1` stores `x]y` rather
		// than refusing: the operand's scan is what is under test.
		sem.SubscriptIsAQuotingContext = Yes
		sem.OperandSubscriptQuoting = q
		r.Semantics = &sem
	})
}

// A builtin's operand is a *string*, so the quoting the shell already took
// off the word reaches the builtin's own scan — and that scan stopped at the
// first `]` however it was written. The element a script meant to clear
// survived, silently, in the dialect that quotes (#3049).
func TestAnOperandSubscriptEndsWhereTheDialectSaysItEnds(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		quotes OperandSubscriptQuotingPolicy
	}{
		{"single quotes", `typeset -A a; a['x]y']=1; unset "a['x]y']"; echo "n=${#a[@]}"`, OperandSubscriptEveryQuote},
		{"double quotes", `typeset -A a; a["x]y"]=1; unset "a[\"x]y\"]"; echo "n=${#a[@]}"`, OperandSubscriptEveryQuote},
		{"a backslash", `typeset -A a; a[x\]y]=1; unset "a[x\]y]"; echo "n=${#a[@]}"`, OperandSubscriptBackslashQuotes},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runOperandQuoting(t, c.quotes, c.src)
			if !strings.Contains(out, "n=0") {
				t.Errorf("quoting: got %q, want the element removed", out)
			}
			// The same operand where the dialect reads no quoting: the
			// subscript ends inside the key, the operand is not one name and
			// one subscript, and the element stands.
			if out, _ := runOperandQuoting(t, OperandSubscriptQuotesNothing, c.src); strings.Contains(out, "n=0") {
				t.Errorf("no quoting: got %q, want the element left standing", out)
			}
		})
	}
}

// The backslash is the rung between the two: the dialect that reads it and
// not the quotes removes the one and refuses the others.
func TestTheBackslashIsItsOwnRungOnTheOperandQuotingLadder(t *testing.T) {
	const quoted = `typeset -A a; a['x]y']=1; unset "a['x]y']"; echo "n=${#a[@]}"`
	if out, _ := runOperandQuoting(t, OperandSubscriptBackslashQuotes, quoted); strings.Contains(out, "n=0") {
		t.Errorf("got %q, want a single-quoted `]` still to end the subscript", out)
	}
}

// The quoting is how the subscript was written and never part of the key, so
// an operand that kept its quotes would name an element nothing has. The
// control is the plain spelling, which must be untouched under every answer.
func TestAPlainOperandSubscriptIsUnchangedByTheQuotingAnswer(t *testing.T) {
	for _, q := range []OperandSubscriptQuotingPolicy{
		OperandSubscriptQuotesNothing, OperandSubscriptBackslashQuotes, OperandSubscriptEveryQuote,
	} {
		out, _ := runOperandQuoting(t, q, `typeset -A a; a[k]=1; unset "a[k]"; echo "n=${#a[@]}"`)
		if !strings.Contains(out, "n=0") {
			t.Errorf("%v: got %q, want the plain key removed", q, out)
		}
		out, _ = runOperandQuoting(t, q, `a=(1 2 3); unset "a[1]"; echo "n=${#a[@]}"`)
		if !strings.Contains(out, "n=2") {
			t.Errorf("%v: got %q, want the indexed element removed", q, out)
		}
	}
}
