// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// With `rcquotes` on, the forms that choose **single quotes** write a literal
// quote as a doubled one rather than closing, escaping and reopening — which
// is the only spelling available there, a backslash being an ordinary
// character inside a single-quoted run.
//
// This is the *write* side of the option; the read side is #4591 and is what
// makes the two compose: a value this shell can now build out of a doubled
// quote is a value it has to be able to write back. Measured on zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26; every row runs in both
// states and the *off* state is the control (#4625).
func TestRcQuotesDecidesHowTheSingleQuotingFormsWriteAQuote(t *testing.T) {
	for _, tc := range []struct{ name, expr, on, off string }{
		{"qq", `${(qq)v}`, `'p''q'`, `'p'\''q'`},
		{"q+", `${(q+)v}`, `'p''q'`, `'p'\''q'`},
		// The three forms that are not single-quoted spellings, and the
		// backslash form, which writes the quote outside any quoting. Each is
		// byte-identical in both states, and they are the rows that say the
		// option reaches the choice of quote rather than every quoter.
		{"q", `${(q)v}`, `p\'q`, `p\'q`},
		{"qqq", `${(qqq)v}`, `"p'q"`, `"p'q"`},
		{"qqqq", `${(qqqq)v}`, `$'p\'q'`, `$'p\'q'`},
		{"q-", `${(q-)v}`, `p\'q`, `p\'q`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "v=\"p'q\"\nsetopt rcquotes\nprint -r -- " + tc.expr +
				"\nunsetopt rcquotes\nprint -r -- " + tc.expr + "\n"
			out, st := runZsh(t, t.TempDir(), src)
			if want := tc.on + "\n" + tc.off + "\n"; st != 0 || out != want {
				t.Errorf("out %q status %d, want %q", out, st, want)
			}
		})
	}
}

// The listings write through the same decision, which is what keeps a value
// from being spelled one way by `typeset -p` and another by `${(qq)}`.
func TestRcQuotesReachesTheListings(t *testing.T) {
	const src = `v="p'q"
setopt rcquotes
typeset -p v
alias al="it's"
alias al
typeset -A m=(["k'1"]="v'2")
typeset -p m
unsetopt rcquotes
typeset -p v`
	out, st := runZsh(t, t.TempDir(), src)
	want := "typeset v='p''q'\n" +
		"al='it''s'\n" +
		"typeset -A m=( ['k''1']='v''2' )\n" +
		`typeset v='p'\''q'` + "\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// The shapes are different rather than one escape substituted for another,
// and a value that begins or ends with a quote is what says so. `(qq)` wraps
// whatever it is given, so with the option off a lone quote comes back as an
// empty pair, the escaped quote and another empty pair — and with it on as
// one pair holding a doubled quote, four characters that read back as one.
func TestRcQuotesWritesOnePairAroundTheWholeValue(t *testing.T) {
	for _, tc := range []struct{ name, value, on, off string }{
		{"a lone quote", `\'`, `''''`, `''\'''`},
		{"a trailing quote", `x\'`, `'x'''`, `'x'\'''`},
		{"a leading quote", `\'x`, `'''x'`, `''\''x'`},
		{"two quotes together", `a\'\'b`, `'a''''b'`, `'a'\'''\''b'`},
		{"nothing at all", `''`, `''`, `''`},
		{"and a value with no quote in it", `'a b'`, `'a b'`, `'a b'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "v=" + tc.value + "\nsetopt rcquotes\nprint -r -- ${(qq)v}\n" +
				"unsetopt rcquotes\nprint -r -- ${(qq)v}\n"
			out, st := runZsh(t, t.TempDir(), src)
			if want := tc.on + "\n" + tc.off + "\n"; st != 0 || out != want {
				t.Errorf("out %q status %d, want %q", out, st, want)
			}
		})
	}
}

// The two sides compose, which is the reason this one was worth closing
// beside the read side: a value built from a doubled quote is written back
// with one.
func TestTheTwoHalvesOfRcQuotesCompose(t *testing.T) {
	const src = "setopt rcquotes\nw=$(print -r -- 'x''y')\nprint -r -- \"[$w]\"\nprint -r -- ${(qq)w}\n"
	out, st := runZsh(t, t.TempDir(), src)
	if want := "[x'y]\n'x''y'\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
