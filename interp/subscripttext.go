// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"

	"github.com/blairham/sh/syntax"
)

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

// operandSubscriptText is the subscript a *builtin's* operand names once the
// round has been made, where the dialect makes one and the session has not
// asked it to stop.
//
// The same shape as declarationSubscriptText one file along, and the same
// round, with one thing added: a switch. The three surfaces this serves are
// the ones bash lets a script turn off by name — `shopt -s assoc_expand_once`
// and its synonym — where a declaration's operand and `[[ -v ]]` keep
// rounding whatever the option says. See
// Runner.ExpandsAnOperandsSubscriptAgain.
//
// The axis is asked *before* the switch is read and only where a round could
// change the text at all, which keeps the order right in both directions: a
// dialect that does not round is never asked about an option it has no name
// for, and a session that turned the round off does not record an answer to
// an axis it then ignores.
func (r *Runner) operandSubscriptText(base, sub string, axis Answer, what string) string {
	if !subscriptTextCouldExpand(sub) {
		return sub
	}
	if !r.ask(axis, what) {
		return sub
	}
	if !r.ExpandsAnOperandsSubscriptAgain() {
		return sub
	}
	// Reassembled into the whole `name[sub]` for expandedSubscriptText's
	// reason: Runner.reference is the one route from a resolved text to the
	// parameter expansion it spells, and going through it is what makes
	// every site's second round the same round.
	if key, again := r.expandedSubscriptText(base + "[" + sub + "]"); again {
		return key
	}
	return sub
}

// operandBracketsWereLexed reports whether the command now running wrote this
// operand with its brackets outside every quoting construct, so the parser
// lexed the subscript and the shell expanded it once before the builtin saw
// anything.
//
// Asked by text rather than by position for Runner.lexedSubscriptOperands'
// reason: an operand reaches a builtin as a string, and the argv slot it
// stood in does not survive the option scan and the name filtering in front
// of it.
func (r *Runner) operandBracketsWereLexed(operand string) bool {
	return slices.Contains(r.lexedSubscriptOperands, operand)
}

// outputOperandBracketsAreLexed is operandBracketsWereLexed for the operand a
// builtin **writes through** — `printf -v a[$k]`, `read a[$k]` — and it is
// asked only while the second round is turned off.
//
// The two halves are one fact. bash refuses `read a[$k]` with `k="80's"` as a
// bad name because it expands the text `a[80's]` again and the round meets a
// quote that never closes; with `shopt -s assoc_expand_once` there is no
// round, and the same operand fills the key `80's`. So the refusal is a
// *consequence* of the round rather than a check beside it, and the state in
// which the brackets are the parser's and nothing will re-read what is
// between them is the state in which the expansion is simply the key —
// exactly as it always is for `unset`, which makes no round here at all.
//
// Measured 2026-09-22 on bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `shopt -s assoc_expand_once` and a table declared in front of it:
//
//	k=']';    printf -v A[$k] r   stores the key `]`
//	k='[';    read A[$k] <<< l    stores the key `[`
//	k="80's"; printf -v A[$k] v   stores the key `80's`
//	          declare A[$k]=Z     still `not a valid identifier`
//
// The last row is why this is the output operand's and not a rule about
// operands: a declaration's subscript rounds under an axis of its own and
// bash's option does not move it, which Runner.ExpandsAnOperandsSubscriptAgain
// already records.
//
// With the round on — the default everywhere, and the only state four of the
// five dialects have — this is false and the operand is read as the text it
// is, which is what every one of those columns was measured doing.
func (r *Runner) outputOperandBracketsAreLexed(operand string) bool {
	return !r.ExpandsAnOperandsSubscriptAgain() && r.operandBracketsWereLexed(operand)
}

// wordBracketsAreLexed reports whether a word carries a `[` and a later `]`
// that both stand in **unquoted** literal text, which is what makes the
// subscript between them a word the parser read rather than a text a builtin
// is handed whole.
//
// Read off the spans, because that is the only place the fact survives:
// `unset m[$k]` and `unset "m[$k]"` come to the same string and name
// different elements. See Runner.subscriptOperandRead for the rows.
// **One** pair, and the closing bracket is the word's last byte. A chain —
// `unset a[1][2]` — wrote two pairs of its own, so the text between the
// first `[` and the last `]` is not one subscript and the operand is left to
// the reading that refuses it. The pair a lexed operand *does* have may hold
// anything at all between the brackets, since whatever is there arrived from
// an expansion: `unset a[$k]` with `k=']'` is one written pair and a key made
// of the other bracket.
func wordBracketsAreLexed(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	opens, closes, endsThere := 0, 0, false
	for _, span := range w.Spans {
		if span.Kind != syntax.Literal || span.Quoting != syntax.Unquoted {
			endsThere = false
			continue
		}
		for i := 0; i < len(span.Value); i++ {
			switch span.Value[i] {
			case '[':
				opens++
				endsThere = false
			case ']':
				closes++
				endsThere = i == len(span.Value)-1
			default:
				endsThere = false
			}
		}
	}
	return opens == 1 && closes == 1 && endsThere
}
