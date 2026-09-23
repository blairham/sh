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
// raw is the subscript as the operand carried it and sub the key its quoting
// comes off to. The round is made over the **raw** text, because the quoting
// is what says which expansions in it are performed and comes off with them:
// an apostrophe the operand carried protects a `$` from this round and is gone
// from the key either way, and removing it first left the `$` to expand
// (#4254). See Runner.subscriptOperandParts for the rows.
//
// The cheap test is still asked of the key, so the question is put exactly
// where it was: a subscript whose quoting is all the round would take off
// reaches the same key both ways, and an axis asked there could not change an
// answer.
func (r *Runner) operandSubscriptText(base, raw, sub string, axis Answer, what string) string {
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
	if key, again := r.expandedSubscriptText(base + "[" + raw + "]"); again {
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

// wordSubscriptIsSourceClosed reports whether this operand's subscript was
// **closed in the source**: the word carries exactly one unquoted `[` and one
// unquoted `]`, the `]` is followed immediately by the assignment operator,
// and everything in between — quoted or expanded — is the subscript.
//
// The sibling of wordBracketsAreLexed, and a different question. That one
// asks about an operand that is a subscript and nothing else (`unset a[k]`);
// this one asks about `a[k]=v`, where the brackets are followed by an
// operator and a value.
//
// It exists because two spellings that are the same string by the time a
// builtin sees them are **not** the same declaration: `declare m['foo[bar']=v`
// and `declare m[foo[bar]=v` both arrive as `m[foo[bar]=v`, and with
// `shopt -s assoc_expand_once` bash takes the first and refuses the second.
// The quoting is the only thing that tells them apart, and it is gone from the
// string — so the fact has to be carried from the word. See
// Runner.subscriptClosedInTheSource.
//
// A **backslash-quoted** bracket counts for nothing here, which is the lexer's
// doing rather than this scan's: `\[` is its own span with its own quoting, so
// the same "unquoted literal only" rule that steps over an apostrophe steps
// over it. Measured 2026-09-23 on bash 5.3.20 with the option set,
// `declare m[foo\[bar]=v` stores the key `foo[bar` exactly as the quoted
// spellings do.
func wordSubscriptIsSourceClosed(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	opens, closes, closed := 0, 0, false
	for n, span := range w.Spans {
		if span.Kind != syntax.Literal || span.Quoting != syntax.Unquoted {
			continue
		}
		for i := 0; i < len(span.Value); i++ {
			switch span.Value[i] {
			case '[':
				if opens > 0 || closes > 0 {
					// A second `[`, or one after the subscript closed:
					// neither is the shape this answers for.
					return false
				}
				if !isPlainName(spanHead(w, n, i)) {
					return false
				}
				opens++
			case ']':
				if opens != 1 || closes > 0 {
					return false
				}
				closes++
				// The operator has to stand right here, in this same
				// unquoted run: a quotation between the `]` and the `=`
				// makes the operator a quoted character and the word an
				// ordinary one.
				rest := span.Value[i+1:]
				closed = strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "+=")
			}
		}
	}
	return opens == 1 && closes == 1 && closed
}

// spanHead is the word's text before byte i of span n, which is a name only
// when every byte of it was written as unquoted literal text — the `[` of a
// subscripted operand stands after a name and after nothing else.
func spanHead(w *syntax.Word, n, i int) string {
	var b strings.Builder
	for k, s := range w.Spans {
		if k == n {
			b.WriteString(s.Value[:i])
			return b.String()
		}
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			return ""
		}
		b.WriteString(s.Value)
	}
	return b.String()
}

// subscriptClosedInTheSource reports that this operand's key ends where the
// **source** ended it rather than where a scan of the expanded text would,
// which is the reading `shopt -s assoc_expand_once` asks for.
//
// The option is one switch and this is its fourth surface. The other three
// are the second expansion round a subscript-as-text gets — see
// Runner.ExpandsAnOperandsSubscriptAgain, whose doc says a declaration's
// operand is out of reach of it, and that is still true: the *key* a
// declaration finds is unchanged by this, and what moves is where the
// subscript is taken to end.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over a script file with standard input on the null device, against bash
// 5.3.20 and bash 5.3.15 in the digest-pinned image the suite is graded in,
// which agree. Fourteen spellings of `declare m[…]=v` with a table declared
// in front of them, each run under the option set and unset:
//
//	                     option unset            option set
//	m['foo[bar']=v       not a valid identifier  the key `foo[bar`
//	m["foo[bar"]=v       not a valid identifier  the key `foo[bar`
//	m[foo\[bar]=v        not a valid identifier  the key `foo[bar`
//	m[foo[bar]=v         not a valid identifier  not a valid identifier
//	m['a[b][c]']=v       the key `a[b][c]`       not a valid identifier
//	m['a[b]c']=v         the key `a[b]c`         not a valid identifier
//	m['a]b']=v           not a valid identifier  not a valid identifier
//	m['a]=b']=v          the key `a`             the key `a`
//	m['[']=v             not a valid identifier  the key `[`
//	m['[[']=v            not a valid identifier  the key `[[`
//	m['x y']=v           the key `x y`           the key `x y`
//
// The two columns **swap** on two of the rows, which is what rules out a rule
// that only loosens or only tightens. With the option unset the subscript is
// found by counting brackets in the text the quoting came off of, so
// `a[b][c]` is balanced and is a key while `foo[bar` is not; with it set the
// source's own closer is taken and the key is then whatever stands before the
// **first** `]`, so `foo[bar` is a key and `a[b][c]` runs past its closer
// into a `[`.
//
// Unreachable in every other dialect, which is why there is no axis: the
// option is bash's alone and the rest of the panel cannot be asked to turn it
// on. Its default is the unset column, so a script that says nothing gets the
// reading this shell already had.
func (r *Runner) subscriptClosedInTheSource(name string) bool {
	if r.ExpandsAnOperandsSubscriptAgain() {
		return false
	}
	switch r.inBuiltin {
	case "readonly", "export":
		// Measured with the rest, and the reason it is a list of two rather
		// than a rule about them: these two answer `readonly r1['a[b']=2`
		// with ``r1[a[b]=2': not a valid identifier`` under the option set
		// **and** unset, where the same operand under `declare` is a key
		// with it set. So the option does not reach their reading of an
		// operand, and a shell that let it reach would shorten the word they
		// quote back.
		return false
	}
	return slices.Contains(r.sourceClosedSubscriptOperands, name)
}

// recordASourceClosedSubscript adds the operand just appended to argv, where
// its subscript was closed in the source.
//
// Called from the two branches that expand a declaration's operand — the
// plain one and the appending one — rather than from the general word loop
// below them, because both of those return before it: an operand carrying a
// value never reaches the place wordBracketsAreLexed is asked.
//
// The name half alone, without the value, since that is the text the reader
// is handed. See Runner.sourceClosedSubscriptOperands.
func recordASourceClosedSubscript(at []string, w *syntax.Word, argv []string) []string {
	if len(argv) == 0 || !wordSubscriptIsSourceClosed(w) {
		return at
	}
	// Through declarationOperand rather than a cut of its own, so that the
	// text recorded is the text the reader is handed: an **appending**
	// operand's name has the `+` taken off, and a list keyed on the other
	// spelling would miss `declare m['foo[bar']+=v` entirely.
	name, _, _, _ := declarationOperand(argv[len(argv)-1])
	return append(at, name)
}
