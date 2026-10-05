// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(f)` subscript flag — `${v[(f)2]}` — changes what a subscript on a
// *string* counts through: its lines rather than its characters. It is the
// one letter in the subscript group that says what the units are rather than
// how a search over them runs, and it is a different construct from the `(f)`
// that splits an *expansion* on newlines: `${(f)v}` is a list of fields and
// `${v[(f)2]}` is one line of a scalar.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), 2026-09-25, `-f`, the
// one shell in the panel with the construct. `v=$'aa\nbb\ncc'` throughout
// unless another value is named:
//
//	${v[(f)2]}       bb       the second line
//	${v[(f)9]}       cc       past the last clamps to the last
//	${v[(f)0]}       aa       and below the first clamps to the first
//	${v[(f)-1]}      cc       a negative counts back from the last
//	${v[(f)-9]}      aa       clamped at that end too
//	${#v[(f)2]}      2        what comes back is the line, so this is its
//	                          length and not a count of lines
//	s=abc; ${s[(f)1]}   abc   a string with no newline in it is one line
//	e=; ${e[(f)1]}      ``    and an empty one has no line to answer with
//
// Three things are *not* this construct, each measured rather than assumed:
//
//	a=(p q r); ${a[(f)2]}   q    an array counts elements whatever the group
//	a=(p q r); ${a[(f)9]}   ``   says — no clamp, so this is the ordinary
//	                             reading and not a line reading that agrees
//	m=(k1 v1); ${m[(f)k1]}  v1   and a table's subscript is still a key
//
// # Which lines there are
//
// Not a split on every newline. A **run** of newlines separates one line from
// the next, a run at the *front* of the value separates nothing, and a run at
// the *end* leaves exactly one empty line behind however long it is.
// Measured, reading the first two lines and the last of each value:
//
//	$'\na\nb'      a b      a leading newline does not make an empty line
//	$'\n\na\nb'    a b      nor does a run of them
//	$'a\n\nb'      a b      nor does a run in the middle
//	$'a\nb\n'      a b ``   a trailing newline makes one empty line
//	$'a\nb\n\n'    a b ``   and a run of them still makes one
//	$'\n'          ``       which is the whole of what this value has
//
// That is how `${v[(f)-1]}` is `c` on `$'a\nb\nc'` and empty on `$'a\nb\n'`,
// and how `${v[(f)3]}` is empty on the second where a clamp to the last line
// would have answered `b`.
//
// # What a range does, and why the two ends are not the same question
//
// This is the row the issue asked to be measured in its own right, and it is
// not what the single-index rows imply. A `(f)` subscript names a **character
// position**, and only a subscript written without a comma reads the whole
// line at it. Measured with `v=$'aaa\nbbb\nccc'`, whose lines begin at
// characters 1, 5 and 9:
//
//	${v[(f)1,2]}    aa        characters 1 through 2, not lines 1 through 2
//	${v[(f)2,2]}    ``        the start is character 5 and the end is 2
//	${v[(f)2,7]}    bbb       5 through 7
//	${v[(f)2,-1]}   bbb\nccc  5 through the last character
//	${v[(f)5,-1]}   ccc       a start past the last line clamps to its line
//
// So the *first* end resolves to where its line begins and the *second* end
// is an ordinary character index — unless it carries a group of its own, and
// then it resolves to where its line **ends**:
//
//	${v[1,(f)2]}        aaa\nbbb   through the end of line 2
//	${v[(f)2,(f)2]}     bbb        one line, spelled as a pair
//	${v[1,(fr)ccc]}     the whole  a search in that end ends at its line too
//
// The trailing empty line is the exception and is measured rather than
// derived: on `$'aa\nbb\n'` — six characters, with that line beginning at
// character 7 — `${v[1,(f)3]}` is `aa\nbb`, which ends at character 5 where
// the *previous* line ended. A line with no characters in it never moved the
// end, so scalarLines records the one it inherited.
//
// # A search over lines
//
// The four selecting letters run over the lines as they run over an array's
// elements — a whole-line pattern match, with `(e)`, `(n:expr:)` and
// `(b:expr:)` read as they are there — and what they *substitute* is the one
// difference: `i` and `I` answer with the character position the line begins
// at rather than with its number. Measured:
//
//	${v[(fr)bb]}     bb   the matching line
//	${v[(fR)*b*]}    bb   and the last such line
//	${v[(fi)bb]}     4    where that line begins, not `2`
//	${v[(fI)*]}      7    so the last line's position, not the line count
//	${v[(fi)b]}      0    the match is the whole line, so this one misses
//	${v[(fr)bb,-1]}  bb\ncc   which is that position read as a range's start
//
// The misses are the half that would have been guessed wrong, and there are
// three of them rather than one. A walk that ran and matched nothing answers
// **0** — the position no line begins at — where an array's forward search
// answers one past its last element. Only a walk that never ran keeps the
// array's answers: `(b:expr:)` above the last line leaves the forward letters
// one past the last *line* and the backward ones one before the first, and
// below the first line both go backward. Measured on `$'aa\nbb\ncc\ndd'`:
//
//	${v[(fi)zz]}        0   a walk that ran and found nothing
//	${v[(fib:3:)bb]}    0   including one that began past its match
//	${v[(fib:9:)bb]}    5   a start above the lines, forward: 1 + 4 lines
//	${v[(fIb:9:)bb]}    0   a start above the lines, backward
//	${v[(fib:-9:)bb]}   0   and a start below them, either way
//	e=; ${e[(fi)x]}     0   a value with no lines has no position at all

// subscriptCountsLines reports whether a subscript's flag group makes a
// string's lines the units it counts through.
//
// `(w)` is the same reading with another separator, and `(f)` is the vendor
// manual's shorthand for `(pws:\n:)`, so one walk answers both: see
// Runner.wordSeparator for what `(w)` separates on.
func subscriptCountsLines(g *syntax.SubscriptFlags) bool {
	return g != nil && strings.ContainsAny(g.Flags, "fw")
}

// wordSeparator is how many units of chars, starting at i, are one separator
// of the words a `(f)` or `(w)` subscript counts through — 0 where none
// starts there.
//
// `(f)` separates on a newline. `(w)` separates on the string an `(s:…:)`
// argument names, read with print escapes only where a `p` was written in
// front of the `s`, and otherwise on any one character of `$IFS`. Measured
// 2026-10-02 on zsh 5.9.2 (`-f`, `LC_ALL=C`), which runs a separator the way
// it runs a newline — a leading run separates nothing, a run in the middle
// is one separator, a trailing run leaves one empty word — with
// `s=$'a\tb c'`:
//
//	${s[(w)2]}           b        a tab and a space are both in $IFS
//	${s[(ws: :)2]}       c        the argument replaces $IFS
//	${s[(pws:\t:)2]}     b c      an escape read as one, behind a `p`
//	${s[(wps:\t:)2]}     b c      the `p` anywhere in front of the `s`
//	${s[(ws:\t:p)2]}     the whole value: two characters found nowhere,
//	                     so one word, and the second clamps to it
//	IFS=:; ${s[(w)1]}    the whole value again
//	s=axybxyc; ${s[(ws.xy.)2]}  b   a separator of several characters
//	s=$'foo\0bar'; ${s[(pws:\0:)2]}  bar
//
// and with `x=:` the argument `$x` is the two characters, `p` or not.
func (r *Runner) wordSeparator(g *syntax.SubscriptFlags) func(chars []string, i int) int {
	if !strings.ContainsRune(g.Flags, 'w') {
		return func(chars []string, i int) int {
			if chars[i] == "\n" {
				return 1
			}
			return 0
		}
	}
	if strings.ContainsRune(g.Flags, 's') {
		sep := g.Sep
		if precededByPrintFlag(g.Flags, 's') {
			sep = r.flagArgEscapes(r, sep)
		}
		units := r.units(sep)
		return func(chars []string, i int) int {
			if len(units) == 0 || i+len(units) > len(chars) {
				return 0
			}
			for j, u := range units {
				if chars[i+j] != u {
					return 0
				}
			}
			return len(units)
		}
	}
	ifs, _ := r.ifs()
	return func(chars []string, i int) int {
		if chars[i] != "" && strings.Contains(ifs, chars[i]) {
			return 1
		}
		return 0
	}
}

// scalarLines is the lines a `(f)` subscript counts through, with the
// character each one begins at and the character after the one it ends at,
// both 0-based over the units the value was handed as.
//
// The scan is what the rules above are stated from: separators are skipped in
// runs, which is what leaves no empty line at the front or in the middle, and
// a run that reaches the end of the value leaves the one empty line there.
// That line begins past the last character and *ends* where the line before
// it ended — see the range rules above, which is the only place the
// difference is visible.
func scalarLines(chars []string, sepAt func([]string, int) int) (lines []string, starts, ends []int) {
	n := len(chars)
	last := 0
	for i := 0; i < n; {
		for i < n {
			k := sepAt(chars, i)
			if k == 0 {
				break
			}
			i += k
		}
		if i == n {
			lines = append(lines, "")
			starts = append(starts, n)
			ends = append(ends, last)
			break
		}
		start := i
		for i < n && sepAt(chars, i) == 0 {
			i++
		}
		lines = append(lines, strings.Join(chars[start:i], ""))
		starts = append(starts, start)
		ends = append(ends, i)
		last = i
	}
	return lines, starts, ends
}

// lineAt is the 0-based line a plain `(f)` subscript names, clamped to the
// lines there are, and -1 where there are none.
//
// The clamp is the whole difference from an ordinary subscript, which answers
// nothing outside the value: `${v[(f)9]}` is the last line and `${v[9]}` is
// empty.
func (r *Runner) lineAt(n, count int) int {
	if count == 0 {
		return -1
	}
	pos := n - r.arrayBase()
	if n < 0 {
		pos = count + n
	}
	if pos < 0 {
		return 0
	}
	if pos >= count {
		return count - 1
	}
	return pos
}

// lineSearchAt is the line a `(f)` search selects, and the character position
// it substitutes for `i` and `I`, in the dialect's base.
//
// The walk is searchElements' — one rule for a search over a list of values,
// whether those are an array's elements or a string's lines — and the three
// misses above are why the kind of miss is read rather than the fact of one.
func (r *Runner) lineSearchAt(g *syntax.SubscriptFlags, search byte, lines []string, starts []int) (at, pos int, found bool) {
	base := r.arrayBase()
	if len(lines) == 0 {
		return 0, base - 1, false
	}
	at, found, miss := r.searchElements(g, search, lines)
	switch {
	case found:
		return at, base + starts[at], true
	case miss == missAbove && search != 'R' && search != 'I':
		return 0, base + len(lines), false
	default:
		return 0, base - 1, false
	}
}

// lineSubscript answers a `(f)` subscript on a string, which is the only
// target the letter says anything about.
func (r *Runner) lineSubscript(e *syntax.ParamExpr, search byte, v string) ([]string, bool) {
	lines, starts, _ := scalarLines(r.units(v), r.wordSeparator(e.IndexFlags))
	if search == 0 {
		idx, ok := r.lineSubscriptValue(e.Subscript())
		if !ok {
			return nil, true
		}
		at := r.lineAt(idx, len(lines))
		if at < 0 {
			return nil, true
		}
		return []string{lines[at]}, true
	}
	at, pos, found := r.lineSearchAt(e.IndexFlags, search, lines, starts)
	if search == 'i' || search == 'I' {
		return []string{itoa(pos)}, true
	}
	if !found {
		return nil, true
	}
	return []string{lines[at]}, true
}

// lineRangeEnd is one end of a range whose group counts lines: where its line
// begins at the front of the pair, and where that line ends at the back.
func (r *Runner) lineRangeEnd(g *syntax.SubscriptFlags, end syntax.SubscriptEnd, search byte, v string, first bool) (int, bool) {
	lines, starts, ends := scalarLines(r.units(v), r.wordSeparator(g))
	base := r.arrayBase()
	if search != 0 {
		at, pos, found := r.lineSearchAt(g, search, lines, starts)
		if first || !found {
			return pos, true
		}
		return base + ends[at] - 1, true
	}
	idx, ok := r.endSubscriptValue(end.Text)
	if !ok {
		return 0, false
	}
	at := r.lineAt(idx, len(lines))
	if at < 0 {
		return base - 1, true
	}
	if first {
		return base + starts[at], true
	}
	return base + ends[at] - 1, true
}

// lineSubscriptValue evaluates the operand behind a `(f)` group, reporting a
// failed expression the way every other subscript does.
//
// The group's own operand rather than the whole subscript text, because the
// letters in front of it are not arithmetic — the same word Subscript()
// hands the ordinary reading once the group has been taken off.
func (r *Runner) lineSubscriptValue(w *syntax.Word) (int, bool) {
	text := r.subscriptText(w)
	n, err := r.subscriptValue(text)
	if err != nil {
		r.diagf("%s\n", r.subscriptFailure(text, err))
		r.expandErr = true
		return 0, false
	}
	return n, true
}

// subscriptTargetIsAString reports whether the name a subscript is written on
// holds a plain string, which is the only target `(f)` says anything about.
//
// A table is asked first because subscriptTarget would read one as its values
// — the letter is nothing to a table, and neither is it to an array.
func (r *Runner) subscriptTargetIsAString(e *syntax.ParamExpr) bool {
	if isAssoc := r.assocDeclared(e.Name); isAssoc {
		return false
	}
	_, scalar, held := r.subscriptTarget(e)
	return held && scalar
}

// lineTargetSpan is the character span a `(f)` subscript names on the **left**
// of a store: where the line it selected begins, and where it ends.
//
// Both ends come from one resolve of the line, which is the whole of why this
// answers a pair rather than a subscript. The read side already resolves the
// same line — see lineSubscript — and the write's job is only to say which
// characters it covers, so that the value replaces the line instead of
// landing inside it.
//
// Measured 2026-09-25 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, the
// one shell in the panel with the construct, with `v=$'aa\nbb\ncc'` each
// time:
//
//	v[(f)2]=ZZ       aa\nZZ\ncc     the line replaced, not a character
//	v[(f)2]+=XX      aa\nbbXX\ncc   and joined at its end
//	v[(f)4]=ZZ       aa\nbb\nZZ     past the last clamps as the read does
//	v[(f)0]=ZZ       ZZ\nbb\ncc     and below the first clamps too
//	v[(fr)bb]=ZZ     aa\nZZ\ncc     a search names the line it matched
//	unset 'v[(f)2]'  aa\n\ncc       the line taken out, separators kept
//	unset 'v[(f)3]'  aa\nbb\n
//
// A **search that missed keeps the refusal**, which is measured rather than
// conservative: that shell answers `v[(fr)zz]=QQ` with `assignment to invalid
// subscript range` — the shape the `R` miss is already refused for — and
// leaves `v[(fi)zz]=QQ` holding `aa\nbb\ncQQaa\nbb\ncc`, a duplicated value
// that is an artifact of its own span arithmetic rather than a statement
// about the construct.
//
// An **array** is not a string, so `a[(f)2]=ZZ` is the ordinary write and
// never reaches this: the caller asks subscriptTargetIsAString first.
func (r *Runner) lineTargetSpan(e *syntax.ParamExpr, refuse func(flag, where string)) (flaggedTarget, bool) {
	g := e.IndexFlags
	v, _ := r.getVar(e.Name)
	lines, starts, ends := scalarLines(r.units(v), r.wordSeparator(g))
	base := r.arrayBase()
	var at int
	if search := lastOf(g.Flags, searchSubscriptFlags); search != 0 {
		found := false
		at, _, found = r.lineSearchAt(g, orderedSearchLetter(search), lines, starts)
		if !found {
			refuse("f", " where a search over the lines of a string matched nothing")
			return flaggedTarget{}, false
		}
	} else {
		idx, ok := r.lineSubscriptValue(g.Arg)
		if !ok {
			return flaggedTarget{}, false
		}
		at = r.lineAt(idx, len(lines))
	}
	if at < 0 {
		// No lines at all, so there is no span to name and nothing the
		// clamp could have reached.
		refuse("f", " where the string has no lines")
		return flaggedTarget{}, false
	}
	return flaggedTarget{from: base + starts[at], to: base + ends[at] - 1, span: true}, true
}
