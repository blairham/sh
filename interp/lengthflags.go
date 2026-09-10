// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The expansion flags that change what `${#…}` counts: `c` counts characters,
// `w` counts words and `W` counts words including the empty ones. They are
// one file because they are one question — none of them does anything to a
// value, and `${(c)a}` and `${(@c)a}` are the plain expansion — so what they
// modify is the length step and nothing else.
//
// Measured on zsh 5.9.2 under the C locale, with `a=(abc de f)` and
// `v="a b  c"`:
//
//	${#a}      3    elements, which is the answer without any of these
//	${(c)#a}   8    the characters of `abc de f`, separators counted
//	${(w)#a}   3    the words in each element, added up
//	${(W)#a}   3    and the same, there being no empty ones
//	${#v}      6    the characters of the scalar
//	${(w)#v}   3    its words
//	${(W)#v}   4    and the empty one between the two spaces
//
// **The length is taken before the double-quoted join, which is not where
// the rule numbers put it.** `"${(U)#a}"` is 3 and not 8, and `"${(Uj.-.)#a}"`
// is 3 as well, so a `j` separator does not reach the count either — `c` is
// the only flag that reads one.

// lengthFlags are the letters this step answers.
const lengthFlags = "cwW"

// lengthFlag is the one of them in force: the **last** written wins, which is
// measured rather than assumed. `${(cw)#v}` on `a b` is 2, which is `w`'s
// answer, and `${(wc)#v}` is 3, which is `c`'s; `${(Wc)#v}` and `${(cW)#v}`
// split the same way round. Zero means none was written.
func lengthFlag(e *syntax.ParamExpr) byte {
	var last byte
	for _, c := range e.Flags {
		if strings.ContainsRune(lengthFlags, c) {
			last = byte(c)
		}
	}
	return last
}

// flaggedLength answers `${#…}` for an expansion carrying a flag group: the
// modifier's count where one was written, and otherwise the plain answer —
// the number of elements for a list and the length of the value for a scalar.
func (r *Runner) flaggedLength(e *syntax.ParamExpr, words []string, isList bool) int {
	switch lengthFlag(e) {
	case 'c':
		return r.countCharacters(e, words)
	case 'w':
		return r.countWords(e, words, false)
	case 'W':
		return r.countWords(e, words, true)
	}
	if isList {
		return len(words)
	}
	return r.stringLength(words[0])
}

// countCharacters is `c`: the characters of the words joined, separators
// included, so `(abc de f)` is 8 rather than 6.
//
// **The separator is a space, and not `$IFS`'s first character** — measured,
// `IFS=:` leaves the same array at 8, which is the reading a shared
// `flagJoinSep` would have got wrong. A `j` argument does replace it:
// `${(cj.--.)#a}` is 10.
func (r *Runner) countCharacters(e *syntax.ParamExpr, words []string) int {
	sep := " "
	if strings.ContainsRune(e.Flags, 'j') {
		sep = e.JoinSep
	}
	n := 0
	for _, w := range words {
		n += r.stringLength(w)
	}
	if len(words) > 1 {
		n += (len(words) - 1) * r.stringLength(sep)
	}
	return n
}

// countWords is `w` and `W`: the words in each element, added up. An array is
// counted a element at a time and the totals summed rather than joined first
// — measured, `a=(a: :b)` with `(s.:.)` is 3 for `w` and 4 for `W`, where a
// join would have made both of them read one word fewer.
func (r *Runner) countWords(e *syntax.ParamExpr, words []string, keepEmpty bool) int {
	sep, explicit := flagWordSep(e)
	ifs, ifsSet := r.ifs()
	n := 0
	for _, w := range words {
		switch {
		case explicit:
			n += literalWordCount(w, sep, keepEmpty)
		case keepEmpty:
			n += separatedFieldCount(w, ifs, ifsSet)
		default:
			n += ifsWordCount(w, ifs, ifsSet)
		}
	}
	return n
}

// flagWordSep is the separator these two count against: the `s` argument
// where one was written, a newline where `f` was and a NUL where `0` was,
// which is the same choice splitFlagged makes — measured, `${(fw)#v}` on
// `a\nb\n` is 3, so `f` reaches the count exactly as `s` does, and with
// `z=$'a\0\0b'` the NUL flag does too: `${(0w)#z}` is 2 and `${(0W)#z}` is 3,
// the pair that says it is counting fields against a separator rather than
// against `$IFS`. Otherwise the separator is `$IFS`, and that is a different
// rule rather than a different value, which is what the second result says.
//
// The letter written *last* decides, as it does for the split itself, and one
// reading of that rule serves both.
func flagWordSep(e *syntax.ParamExpr) (sep string, explicit bool) {
	i := strings.LastIndexAny(e.Flags, splitFlagLetters)
	switch {
	case i < 0:
		return "", false
	case e.Flags[i] == 's':
		return e.SplitSep, true
	case e.Flags[i] == '0':
		return "\x00", true
	}
	return "\n", true
}

// literalWordCount counts against an explicit separator, which is a string
// split literally rather than a set of characters.
//
// The two halves are measured, and they are not each other's mirror. With
// `(s.:.)`, `W` is every field the split makes — `”` is 1, `':'` is 2,
// `'a::b'` is 3 — while `w` collapses runs of the separator and drops a
// leading empty field but keeps a trailing one: `”` is 1, `':'` is 1,
// `':a:'` is 2, `'a::b'` is 2 and `'a::'` is 2. An empty separator counts
// characters, `${(ws::)#v}` on `abc` being 3, and an empty value is still one
// field either way.
func literalWordCount(w, sep string, keepEmpty bool) int {
	if sep == "" {
		return max(characterCount(w), 1)
	}
	if keepEmpty {
		return strings.Count(w, sep) + 1
	}
	// Runs of text the separator does not claim, plus one for a trailing
	// separator: the field it opens is kept where a leading or interior
	// empty one is not.
	n := 0
	for _, part := range strings.Split(w, sep) {
		if part != "" {
			n++
		}
	}
	if strings.HasSuffix(w, sep) {
		n++
	}
	return max(n, 1)
}

// separatedFieldCount is `W` against `$IFS`: every separator delimits, so
// nothing collapses and nothing is trimmed. `'a b  c'` is 4 and `'  '` is 3;
// an empty value is no field at all, which is where this parts company with
// an explicit separator, and IFS set to empty splits nothing.
func separatedFieldCount(w, ifs string, ifsSet bool) int {
	if w == "" {
		return 0
	}
	if ifsSet && ifs == "" {
		return 1
	}
	n := 1
	for i := range len(w) {
		if strings.IndexByte(ifs, w[i]) >= 0 {
			n++
		}
	}
	return n
}

// ifsWordCount is `w` against `$IFS`: the fields ordinary word splitting
// makes, plus the one a trailing non-whitespace separator opens.
//
// That last clause is the whole of the difference from splitFields, and it is
// measured rather than reasoned: with `IFS=:`, `'a:'` counts 2 where the
// splitting stage yields one field, because a trailing delimiter is absorbed
// there. It is the *run* that decides, not the last byte — `IFS=': '` counts
// `'a: '` as 2 and `'a '` as 1, so a run containing one non-whitespace
// separator opens a field however much whitespace follows it.
func ifsWordCount(w, ifs string, ifsSet bool) int {
	n := len(splitFields(w, ifs, ifsSet))
	if trailingRunSeparates(w, nil, ifs, ifsSet) {
		n++
	}
	return n
}

// trailingRunSeparates reports whether the value's closing run of separators
// holds a non-whitespace one.
//
// It is also the guard on TrailingSeparatorEndsAField, which is why it takes
// the literal mask `read` carries: an escaped separator is data, so it ends
// the run rather than belonging to it, and `read -A` on `a\:` is one field in
// the shell where `a:` is two. A nil mask exempts nothing, which is every
// caller but that one.
func trailingRunSeparates(w string, literal []bool, ifs string, ifsSet bool) bool {
	if ifsSet && ifs == "" {
		return false
	}
	found := false
	for i := len(w) - 1; i >= 0; i-- {
		if literal != nil && literal[i] {
			break
		}
		c := w[i]
		if strings.IndexByte(ifs, c) < 0 {
			break
		}
		if c != ' ' && c != '\t' && c != '\n' {
			found = true
		}
	}
	return found
}
