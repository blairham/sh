// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// operandSubscriptQuoting is which quoting holds a `]` back from ending the
// subscript in a **builtin's operand** — `unset "a['x]y']"`, where the quotes
// reached the builtin because the shell's own quoting came off in front of it.
//
// The parser asks the same question of source text and reads
// syntax.Dialect.SubscriptQuoteProtectsTheClosingBracket for it. The two
// answers are not the same answer, which is why this is a field rather than
// that flag read twice: measured 2026-09-18 from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, storing the key and then naming it back,
//
//	                       unset "a[x\]y]"   "a['x]y']"   "a[\"x]y\"]"
//	bash 5.3.20            removed           removed      removed
//	ksh93u+ 2012-08-01     removed           refused      refused
//	zsh 5.9.2              left standing     refused      refused
//
// where the *read* side has ksh93 quoting with each of the four constructs
// and zsh with the backslash alone.
func (r *Runner) operandSubscriptQuoting() syntax.SubscriptQuoting {
	switch r.sem().OperandSubscriptQuoting {
	case OperandSubscriptEveryQuote:
		return syntax.SubscriptBackslashQuotes | syntax.SubscriptSingleQuotes |
			syntax.SubscriptDoubleQuotes
	case OperandSubscriptBackslashQuotes:
		return syntax.SubscriptBackslashQuotes
	}
	return 0
}

// operandBracketsBalance reports whether text is one `[…]` and nothing beyond
// it, reading the dialect's answer for what a quote does to the `]`.
//
// The quoting is the whole of the change: without it the scan stopped at the
// first `]` however it was written, so `unset "a['x]y']"` was an unbalanced
// operand, was refused as a *name*, and in the column that quotes it was
// refused silently — the element the script meant to clear survived with
// nothing said (#3049). `${a['x]y']}` had learned the same lesson one scanner
// over and this one did not move with it.
//
// lexed is whether the **parser** read these brackets, which decides what an
// unterminated quote between them is: a byte of the key where the subscript
// was a word the shell expanded, and a run to the end of the text where the
// operand arrived as one string. See syntax.SubscriptClosingBracket's
// looseQuotes and Runner.lexedSubscriptOperands.
func (r *Runner) operandBracketsBalance(text string, lexed bool) bool {
	end := syntax.SubscriptClosingBracket(text, r.operandSubscriptQuoting(), lexed)
	return end >= 0 && end == len(text)-1
}

// operandSubscriptUnquoted is the subscript text with the quoting the scan
// above stepped over taken off, so that the key names the element the store
// wrote.
//
// The quoting is part of how the subscript was *written* and never part of
// the key: `typeset -A a; a['x]y']=1` stores `x]y` in every column with a
// table, so an operand that keeps the quotes names an element nothing has.
// Only the constructs this dialect steps over are removed — a quote it does
// not read is an ordinary character of the key, which is what keeps the two
// halves of one answer from disagreeing.
//
// A **substitution** in the text is stepped over whole rather than read: the
// quoting inside `$( … )` is the quoting of the commands in it. See
// substitutionSpan.
func (r *Runner) operandSubscriptUnquoted(sub string) string {
	q := r.operandSubscriptQuoting()
	if q == 0 || !strings.ContainsAny(sub, `\'"`) {
		return sub
	}
	var b strings.Builder
	for i := 0; i < len(sub); i++ {
		if n := substitutionSpan(sub, i); n > 0 {
			// A substitution's own text, copied across whole. The quoting
			// inside `$( … )` belongs to the commands in it and not to the
			// subscript around them, and taking it off here is how a
			// subscript came to be matched against the working directory:
			// `w='a[$(echo "*")]'; unset "$w"` lost the quotes, ran `echo *`
			// instead, and the sentence a script got back named the files in
			// the directory. Measured 2026-09-21, bash 5.3.20 and 3.2.57
			// alike, the quoted body is one asterisk and the unquoted body
			// `a[$(echo *)]` is the listing in both — so the body globs, and
			// what must not happen is this scan deciding for it (#4070).
			b.WriteString(sub[i : i+n])
			i += n - 1
			continue
		}
		switch c := sub[i]; c {
		case '\\':
			if q.Has(syntax.SubscriptBackslashQuotes) && i+1 < len(sub) {
				i++
				b.WriteByte(sub[i])
				continue
			}
			b.WriteByte(c)
		case '\'', '"':
			construct := syntax.SubscriptSingleQuotes
			if c == '"' {
				construct = syntax.SubscriptDoubleQuotes
			}
			if !q.Has(construct) {
				b.WriteByte(c)
				continue
			}
			end := strings.IndexByte(sub[i+1:], c)
			if end < 0 {
				b.WriteByte(c)
				continue
			}
			b.WriteString(sub[i+1 : i+1+end])
			i += end + 1
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// substitutionSpan is the length of the substitution that starts at i, or 0
// where nothing does.
//
// The three spellings a subscript can carry — `$( … )`, `${ … }` and a
// backquoted body — read as one region each so that the scan above steps over
// them rather than through them. Nesting is counted for the brace and paren
// forms, which is what makes `$(( … ))` and `$(f $(g))` one region apiece,
// and a quote inside the body is stepped over as well: `$(echo ")")` closes
// at the last paren and not at the quoted one.
//
// An **unterminated** region is not one. It returns 0, and the byte is read
// as the ordinary character it is — the same standing the scan above gives an
// unterminated quote, and the reason a lone `$` or a stray backquote in a key
// is left exactly where it was written.
func substitutionSpan(text string, i int) int {
	switch {
	case text[i] == '`':
		for j := i + 1; j < len(text); j++ {
			switch text[j] {
			case '\\':
				j++
			case '`':
				return j - i + 1
			}
		}
		return 0
	case text[i] != '$' || i+1 >= len(text):
		return 0
	}
	opener := text[i+1]
	var closer byte
	switch opener {
	case '(':
		closer = ')'
	case '{':
		closer = '}'
	default:
		return 0
	}
	depth := 0
	for j := i + 1; j < len(text); j++ {
		switch c := text[j]; c {
		case opener:
			depth++
		case closer:
			if depth--; depth == 0 {
				return j - i + 1
			}
		case '\\':
			j++
		case '\'', '"':
			if end := strings.IndexByte(text[j+1:], c); end >= 0 {
				j += end + 1
			}
		}
	}
	return 0
}
