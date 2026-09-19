// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A subscript that reaches a construct as *text* rather than as a word.
//
// `a[$k]=v` written on a line of its own never gets here: the parser lexed
// the brackets, so the subscript arrived as a word and was expanded before
// anything looked at it. The text shape is what a construct receives when the
// brackets were quoted — `typeset 'a[$k]'=v` — or when they came out of a
// value, and then the `$k` between them is three characters that nothing has
// expanded yet.
//
// Two of the columns expand such a text a second time and two do not; the
// axes are Semantics.DeclarationOperandExpandsItsSubscript and
// Semantics.ConditionIsSetExpandsAFlatSubscript, each measured where its own
// construct is. This file is the expansion they share, so the two sites
// cannot drift apart over what a second round does.

// expandedSubscriptText is the key a subscripted operand names once the
// subscript it arrived as text has been read as a word and expanded.
//
// operand is the whole `name[sub]`, not the subscript alone, because
// Runner.reference is what turns text into that word: it is the one route in
// this shell from a resolved text to the parameter expansion it spells, and
// going through it means a text the reader would not call a subscripted name
// is declined here exactly as it is declined everywhere else.
//
// The second round is a word expansion with **splitting and globbing off and
// no tilde**, which is measured rather than assumed. On bash 5.3.20 with
// `k='x y'`, `typeset 'm[$k]'=V` stores one key `x y` rather than two; with
// `k='x*y'` and a file `xzy` beside it the key is the literal `x*y`; and an
// element stored under the one character `~` is found by `typeset 'm[~]'`,
// which a tilde expansion would have turned into a home directory. Quote
// removal does run — `m['q']` names `q` — which is what expandWordNoSplit
// already does and why it is what this calls.
//
// An expansion that leaves the subscript **empty** is deliberately not one of
// these. The two columns that expand part company there — measured 2026-09-19
// with `nope` unset, `typeset 'd[$nope]'=Q` is `d[$nope]: bad array
// subscript` at status 1 in one and a written empty key at status 0 in the
// other — so an emptied subscript is a question of its own and is left
// standing as the text it was written as, which is where it stood before
// there was a second round at all.
//
// The second result reports whether the text moved. It is false for every
// subscript that expands to itself, which keeps a caller from asking its axis
// about a key the two readings agree on.
func (r *Runner) expandedSubscriptText(operand string) (string, bool) {
	e, ok := r.reference(operand)
	if !ok || e.Index == nil {
		return "", false
	}
	got := strings.Join(r.expandWordNoSplit(e.Index), "")
	if got == e.IndexText || got == "" {
		return "", false
	}
	return got, true
}

// subscriptTextCouldExpand reports whether a second round could possibly
// change this text, read off the characters and running nothing.
//
// It is the guard that keeps the axes above off the common path — `a[1]`,
// `a[k]` and `a[i+1]` have nothing in them for an expansion to do, so the two
// readings reach the same element and the question is never put. A text that
// passes this test may still expand to itself; that is what
// expandedSubscriptText's second result is for, and the split matters because
// this test runs no substitutions and that one does.
//
// The five characters are the ones that make a word do something: a `$` for a
// parameter or an arithmetic expansion, a backquote for the old spelling of a
// substitution, a backslash for an escape, and either quote for a quoting
// that quote removal will take off again.
func subscriptTextCouldExpand(sub string) bool {
	return strings.ContainsAny(sub, "$`\\'\"")
}
