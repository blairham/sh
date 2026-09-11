// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// subscripting is the grammar these expansions need, named by the construct
// rather than by a shell.
func subscripting(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
}

// emptyParamSub answers the axis, and the associative one beside it so that a
// row about `${m[]}` fails over the subscript rather than over a declaration.
func emptyParamSub(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.EmptyParamSubscriptIsAnError = a
		r.Semantics = &s
	}
}

// TestAnEmptyParameterSubscriptAnswersByAxis is #1763.
//
// `${a[]}` — brackets written with nothing at all between them — expanded to
// the element the empty expression names, at status 0, in every dialect. Five
// of the six columns refuse it and each ends the input; one reads it. So the
// wrong answer was the quiet kind, and the worst row is the scalar: `${s[]}`
// came back with the whole of `s` where the shell had stopped.
//
// Read the axis both ways on the same sources, because the value alone cannot
// tell "refused" from "read": a refusal that produced the element anyway would
// look identical on the accepting side.
func TestAnEmptyParameterSubscriptAnswersByAxis(t *testing.T) {
	const setup = `a=(5 6 7); s=hi; `
	for _, tc := range []struct{ name, src, read string }{
		{"an indexed array", `echo "[${a[]}]"`, "[5]"},
		{"a scalar", `echo "[${s[]}]"`, "[hi]"},
		{"a name nothing declared", `echo "[${nodecl[]}]"`, "[]"},
		// The operator makes no difference in any column, which is why the
		// question is asked where the subscript is read rather than once per
		// operator.
		{"under a length", `echo "[${#a[]}]"`, "[1]"},
		{"under a default", `echo "[${a[]:-d}]"`, "[5]"},
		{"under a trim", `echo "[${a[]%x}]"`, "[5]"},
		{"under a replacement", `echo "[${a[]/5/X}]"`, "[X]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, setup+tc.src+`; echo after`, subscripting, emptyParamSub(No))
			if want := tc.read + "\nafter\n"; out != want || st != 0 {
				t.Errorf("read: %s = %q (status %d), want %q at 0", tc.src, out, st, want)
			}
			out, st = runGrammar(t, setup+tc.src+`; echo after`, subscripting, emptyParamSub(Yes))
			if !strings.Contains(out, "bad substitution") {
				t.Errorf("refused: %s = %q, want a bad substitution", tc.src, out)
			}
			if strings.Contains(out, "after") || st == 0 {
				t.Errorf("refused: %s = %q (status %d), want the input to end", tc.src, out, st)
			}
		})
	}
}

// TestAnEmptyParameterSubscriptIsTheWrittenBracketsAndNotAnEmptyOne: the
// question is asked of the node, before anything is expanded.
//
// A subscript that *arrived* empty is a different construct and the column
// that refuses `${m[]}` accepts it: the subscript text is `$w`, and the
// expansion happens inside the subscript rather than before it. That is the
// opposite of the arithmetic site, where the parameters go in first and an
// empty `$w` really does produce `a[]` — which is why the two sites need two
// axes rather than one.
//
// Blank is not empty either. Whether `${a[ ]}` is the blank expression is
// BlankArithSubscriptIsTheEmptyExpression's question and not this one, and a
// check written on the *expanded* text would have swallowed both rows.
func TestAnEmptyParameterSubscriptIsTheWrittenBracketsAndNotAnEmptyOne(t *testing.T) {
	t.Run("an empty quotation", func(t *testing.T) {
		out, st := runGrammar(t, `s=hi; echo "[${s[""]}]"; echo after`, subscripting, emptyParamSub(Yes))
		if strings.Contains(out, "bad substitution") || !strings.Contains(out, "after") || st != 0 {
			t.Errorf(`${s[""]} = %q (status %d), want it read rather than refused`, out, st)
		}
	})
	for _, tc := range []struct{ name, src, want string }{
		{"through a parameter", `w=; echo "[${a[$w]}]"`, "[5]"},
		{"through a command substitution", `echo "[${a[$(printf "")]}]"`, "[5]"},
		{"blank", `echo "[${a[ ]}]"`, "[5]"},
		// An empty *quotation* is a subscript with two characters in it, not
		// brackets with nothing between them: `${m[""]}` looks up the empty
		// key in the column that refuses `${m[]}`. Asserted as "not refused"
		// rather than on the value, because what an empty key looks up is a
		// separate question this change does not answer.
		// And the control: a subscript that names something is untouched.
		{"a written index", `echo "[${a[1]}]"`, "[6]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `a=(5 6 7); ` + tc.src + `; echo after`
			out, st := runGrammar(t, src, subscripting, emptyParamSub(Yes))
			if want := tc.want + "\nafter\n"; out != want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, want)
			}
		})
	}
}

// TestAnEmptyParameterSubscriptRefusalTakesTheDialectsOwnSentence: four of the
// five refusing columns give their ordinary bad-substitution sentence and one
// has its own, so the wording is a field and the refusal is not.
func TestAnEmptyParameterSubscriptRefusalTakesTheDialectsOwnSentence(t *testing.T) {
	own := func(r *Runner) {
		emptyParamSub(Yes)(r)
		d := Diagnostics{EmptyParamSubscript: "invalid subscript"}
		r.Diagnostics = &d
	}
	out, st := runGrammar(t, `a=(5 6 7); echo "[${a[]}]"; echo after`, subscripting, own)
	if !strings.Contains(out, "invalid subscript") || strings.Contains(out, "bad substitution") {
		t.Errorf("got %q, want the dialect's own sentence and not the substitution one", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the input to end", out, st)
	}
	// And the bad-substitution route names what that sentence names — the
	// quoting run, in the dialect whose wording carries a verb at all.
	out, _ = runGrammar(t, `a=(5 6 7); echo "[${a[]}]"`, subscripting, func(r *Runner) {
		emptyParamSub(Yes)(r)
		d := Diagnostics{
			BadSubstitution:      "%[1]s: bad substitution",
			BadSubstitutionNames: NamesTheQuotingRun,
		}
		r.Diagnostics = &d
	})
	if want := "[${a[]}]: bad substitution"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q in it", out, want)
	}
}

// TestAnEmptyParameterSubscriptUnansweredIsRefused: five columns stop and one
// hands back a value, so there is nothing to fall back on — and the value is
// the half that cannot be guessed, since it is the element the brackets name
// rather than anything about emptiness.
func TestAnEmptyParameterSubscriptUnansweredIsRefused(t *testing.T) {
	out, _ := runGrammar(t, `a=(5 6 7); echo "[${a[]}]"; echo after`, subscripting,
		emptyParamSub(Unspecified))
	if !strings.Contains(out, "a subscript written with nothing in it") ||
		!strings.Contains(out, "no dialect was chosen") {
		t.Errorf("got %q, want the axis named in the refusal", out)
	}
	if strings.Contains(out, "[5]") {
		t.Errorf("got %q, want no value where no dialect answered", out)
	}
}
