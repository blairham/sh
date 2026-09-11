// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(e)` expansion flag: the value the pipeline arrived at is read again as
// shell text, so a `$name`, a `$(cmd)` or a `$((expr))` that was data becomes
// a substitution.
//
// The vendor manual puts it at rule 21, after the splitting, the case
// conversion, the quoting and the ordering, and measurement agrees from three
// directions on zsh 5.9.2, 2026-09-09, with `d=DD` and `sp='a b'`:
//
//	v='$d';   ${(eU)v}        empty      `(U)` made it `$D`, which is unset
//	v='$sp';  ${(es: :)v}     a b        one field: the split ran first
//	v='$sq';  ${(eq)v}        $sq        `(q)` escaped the `$` and `(e)` ate it
//
// The first is the sharpest: a `(e)` that ran before the case conversion would
// answer `DD`, which is what a reader expects and is not what the shell says.
//
// What the re-reading *is* has one more measurement behind it than the manual
// gives, because "re-examined for new parameter substitutions" does not say
// what happens to the text around them. Every row below is measured, with
// `arr=(p q r)`:
//
//	'~'            ~            no tilde expansion
//	'a*'           a*           no filename generation, one field
//	'{x,y}'        {x,y}        no brace expansion
//	"'\$d'"        'DD'         a quote is text, and does not protect
//	'\$d'          $d           a backslash before a `$` does
//	'a\tb'         a\tb         and before anything else is text, both of it
//	'$'            $            a `$` with nothing usable after it is a `$`
//	'$inner'       $deeper      one level only, where inner='$deeper'
//	'$arr'         p q r        three fields, unquoted
//	'$arr' quoted  p q r        one, joined with IFS's first character
//	'$(echo a; echo b)'  a b    two fields: a command substitution splits
//
// Those are exactly the rules a here-document body follows — a backslash is an
// escape only in front of `$`, a backtick, another backslash or a newline;
// quotes are text; nothing globs and nothing brace-expands — laid over the
// ordinary word-building rules for what a substitution's *result* does. So
// this is the here-document reader with the substitutions unquoted, rather
// than a second expander written to the same description, and the two halves
// were confirmed against the options that move them: under `SH_WORD_SPLIT`
// the scalar `$sp` becomes two fields here, and under `GLOB_SUBST` an `a*` in
// the *result* matches, both of which fall out of asking the axes rather than
// deciding here.
//
// `${` with nothing that reads is a bad substitution that stops the script,
// which needs no code here: it is what any unreadable expansion does, reached
// through the same parser.
//
// A value that names its *own* expansion does need code, and is the one place
// this deliberately does not do what the shell does: `v='${(e)v}'` spins there
// until it is killed, measured, and reproducing that in a library is a choice
// between hanging the embedder and overflowing its stack. The depth is bounded
// instead — see maxReevalDepth.

// maxReevalDepth bounds how deep the re-reading may go. See Runner.reevalDepth
// for why there is a bound at all; the number is the one arithValueOf uses,
// because the two are the same hazard and a script that means anything by
// either is nowhere near it.
const maxReevalDepth = 32

// reevalFlagApplies reports whether this group asks for the re-reading.
func reevalFlagApplies(e *syntax.ParamExpr) bool {
	return strings.ContainsRune(e.Flags, 'e')
}

// reevalFlagged reads each word again as shell text and returns the fields
// they came to.
//
// Per word, which is measured: with `b=('$arr' x)` and `arr=(p q r)`,
// `"${(@e)b}"` is four fields — the three the first element's reference
// produced and the `x` — so the flag reaches an element at a time and the
// fields it makes are laid into the result in order.
//
// And the rejoin of rule 23 is per word too, which is the half that is not
// obvious and is the half that has to be measured. Where the expansion has to
// come to a single word, the fields *one* word produced go back together with
// the first character of IFS; the words the pipeline already held stay apart.
// With `arr=(p q r)`, `z='$arr'`, `u='$arr:$arr'` and `v='$arr $arr'`:
//
//	"${(e)z}"        1  [p q r]              one word in, one out
//	"${(@e)z}"       3  [p][q][r]            the `@` keeps the fields
//	${(e)z}          3  [p][q][r]            and so does being unquoted
//	"${(es.:.)u}"    2  [p q r][p q r]       two words in, two out
//	"${(e)=v}"       2  [p q r][p q r]       however the two were made
//	"${(@es.:.)u}"   6                       and `@` keeps all six
//	IFS=-; "${(e)z}" 1  [p-q-r]              IFS and not a space
//	"${(ej:-:)z}"    1  [p q r]              IFS even beside a `j`
//
// Rows four and five are the discriminating ones: a rejoin over the whole
// result would answer both with one field, and one that never joined would
// answer the first with three. The last row is worth keeping because rule 5's
// join takes the `j` separator and this one does not, so reusing flagJoinSep
// here would answer it `p-q-r`.
func (r *Runner) reevalFlagged(e *syntax.ParamExpr, words []string, quoted bool) (out []string, joined, ok bool) {
	if r.reevalDepth >= maxReevalDepth {
		// A value that names its own expansion, which the shell being
		// modeled does not terminate on either — it spins until it is
		// killed, measured on `v='${(e)v}'`. Neither answer is the value of
		// anything, so this one says so and stops rather than reproducing a
		// hang or, worse, overflowing the stack of whatever program embeds
		// this runner.
		r.diagf("${%s}: nested too deeply\n", e.Src)
		r.expandErr = true
		return nil, false, false
	}
	r.reevalDepth++
	defer func() { r.reevalDepth-- }()

	// Whether one word is required, which is the only question rule 23 asks.
	// Unquoted, the fields are what the command line wanted; quoted, they are
	// one word unless the group asked for the fields by name.
	join := quoted && !r.flagKeepsFields(e)
	out = make([]string, 0, len(words))
	for _, w := range words {
		fields, fok := r.reevalText(w)
		if !fok {
			return nil, false, false
		}
		if join {
			out = append(out, strings.Join(fields, ifsFirst(r.ifs())))
			continue
		}
		out = append(out, fields...)
	}
	return out, join, true
}

// reevalText is one word read again.
//
// The spans come from the here-document reader, which is what makes a
// backslash an escape in front of `$`, a backtick and a backslash and text
// everywhere else, and makes a quote a character rather than a quoting. It
// marks every span double-quoted, because a body is one blob of input; here
// the substitutions are unquoted instead, so each of them splits, globs and
// yields fields by the ordinary rules — which is measured, `${(e)v}` on an
// array reference being three fields and on `$(echo a; echo b)` being two.
//
// The literals stay marked as the reader left them. That is not a detail: an
// unquoted literal's metacharacters are live for the *enclosing* word's match,
// and its backslashes are then read a second time by the unescape below, so a
// value of `a\tb` came back as `atb`. Quoted, they are carried through the
// escaped form and arrive as themselves — and the value's own metacharacters
// are not a pattern here anyway, measured: `${(e)v}` on `a*` is `a*`, and it
// matches only when `GLOB_SUBST` makes the *result* of the whole expansion a
// pattern, which is the enclosing expansion's question and not this one's.
func (r *Runner) reevalText(text string) ([]string, bool) {
	// Text that ran out inside an expansion is refused rather than read as
	// the expansion the lexer had to invent to hand it back; see rawSpans.
	spans, ok := r.rawSpans(text)
	if !ok {
		return nil, false
	}
	for i := range spans {
		if spans[i].Kind != syntax.Literal {
			spans[i].Quoting = syntax.Unquoted
		}
	}
	failed := r.expandErr
	fields := r.expandOneWordFields(&syntax.Word{Spans: spans})
	if r.expandErr && !failed {
		return nil, false
	}
	for i, f := range fields {
		fields[i] = globUnescape(f)
	}
	return fields, true
}
