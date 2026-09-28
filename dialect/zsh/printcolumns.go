// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/blairham/sh/interp"
)

// `print -C n` writes the operands in **n columns, filled down**.
//
// The first column takes the first ceil(count/n) words rather than the first
// row, which is what "filled down" means and is the half a reader guesses
// wrongly: `print -C 2 a b c d` is `a  c` over `b  d`, not `a  b` over `c  d`.
//
// # The field width, and the two rows that decide it
//
// Every field but the **last of its row** is padded, and they are all padded
// to the same width: the longest padded word plus two. Measured 2026-09-28 on
// zsh 5.9.2 under `-f` from a script file, with the rows chosen so that the
// three obvious readings part company:
//
//	print -C 2 a b c d              a··c / b··d          width 3
//	print -C 2 aaaa bbbb cccc dddd  aaaa··cccc / …       width 6
//	print -C 2 aaa b cc dddd        aaa··cc / b····dddd  width 5
//	print -C 3 1 22 333 4444 55555  1·····333···55555    width 6
//	                                22····4444
//
// The third row is the one that rules out "the longest word in the whole
// list": `dddd` is four characters and the width is five, because `dddd` sits
// in the last column and is never padded. The fourth rules out "the longest
// word in each column": the width is one number for the whole layout —
// column one holds `1` and `22` and is still six wide, because `4444` in
// column two is.
//
// So it is the longest word among those that get padded, plus two. The issue
// this closes read the first row as "plus one" from a single case where the
// two readings coincide.
//
// **And the same rule settles the edge it left open.** `print -C 5 a b` comes
// back `a··b` — two spaces where "plus one" predicts one — and needs no second
// rule: two words in five columns is one row of two fields, `a` is padded and
// `b` is not, so the width is 1+2.
//
// # The length is a display width, and a control character has none
//
// Not the byte count and not the character count. Measured 2026-09-28, four
// layouts over a first operand of four different shapes, each against
// `cc dd ee`:
//
//	xyz           width 5   three columns of three
//	ab            width 4   two
//	a\tb          width 4   the tab counts **nothing**, so this is two
//	你好          width 6   two wide characters, so this is four
//
// So a tab or a newline in an operand takes no room in the arithmetic while
// still being written, and a wide character takes two. `é` takes one, which
// is what rules out the byte count.
//
// It is the width of the **processed** text and not of the operand as
// written, which the tab row is what settles: `a\tb` is four characters and
// the layout is four wide, so the escape is expanded and then measured.
//
// The width table is [interp.DisplayColumns] — the same one every other width
// question in this engine is answered from, rather than a second one here
// that could come to disagree about a wide character. Taking the control
// characters out first is this layout's own rule and not that table's: that
// table measures a tab as one column, because a prompt draws it as at least
// one.
//
// # What the other letters do
//
// `-C` wins over `-l` and `-N`'s separators, which is what makes it a layout
// rather than a join: measured, `print -l -C 2 a b c` is the same two lines
// `print -C 2 a b c` is. What `-N` still decides is the **row terminator** —
// `print -N -C 2 a b c d` ends each row with a NUL and writes no trailing
// newline — and `-n` decides nothing at all here, the rows keeping their
// separators either way. `-o`, `-r` and `-u` are unchanged: the sort happens
// first, the escapes follow the letter, and the descriptor is where the rows
// go.
func printColumnLines(r *interp.Runner, opts printOptions, words []string) (string, bool, bool) {
	rows := (len(words) + opts.columns - 1) / opts.columns
	if rows <= 0 {
		return "", true, false
	}
	// The **processed** text of every operand first, because that is what the
	// arithmetic is done on: an operand's escapes are expanded and then
	// measured, which is what makes `print -C 2 'a\tb' cc dd ee` four wide
	// rather than six. Two passes for that reason and not for tidiness — a
	// layout that measured the operand as written gets that line wrong and
	// every line without an escape in it right.
	fields := make([]string, len(words))
	for i := range words {
		text, ok, _, refused := printJoined(r, opts, words[i:i+1], "")
		if !ok {
			return "", false, false
		}
		if refused {
			return "", true, true
		}
		fields[i] = text
	}
	width := columnWidth(fields, rows)
	var b strings.Builder
	for row := 0; row < rows; row++ {
		for i := row; i < len(fields); i += rows {
			b.WriteString(fields[i])
			if !lastInItsRow(i, rows, len(fields)) {
				b.WriteString(strings.Repeat(" ", width-columnDisplayWidth(fields[i])))
			}
		}
		b.WriteString(opts.rowTerminator())
	}
	return b.String(), true, false
}

// columnWidth is how wide every padded field is: the longest word outside the
// **last column**, plus two.
//
// The set is by *column* and not by "the words that get padded", and one
// layout is what tells the two apart. `print -C 3 1 22 333 4444 55555` fills
// three columns down — `1`,`22` then `333`,`4444` then `55555` — and comes
// back six wide:
//
//	1·····333···55555
//	22····4444
//
// `4444` is the last field of its *row*, so it is never padded; and it is in
// column two, so its four characters are what the width is built from. A rule
// written on the padded words alone makes that layout five wide and every
// other measured one right, which is exactly the shape of a rule that agrees
// almost everywhere.
//
// The last column is excluded rather than the longest word being taken from
// the whole list, which the third row of printColumnLines' table settles:
// `print -C 2 aaa b cc dddd` is five wide with a four-character `dddd` in it.
func columnWidth(words []string, rows int) int {
	last := (len(words) - 1) / rows
	width := 0
	for i, w := range words {
		if i/rows >= last {
			continue
		}
		if n := columnDisplayWidth(w); n > width {
			width = n
		}
	}
	return width + columnGap
}

// columnGap is the two spaces that follow the longest counted word.
//
// Named because the number is the whole of what four measured layouts pin
// down and a `1` here is the reading the issue that filed this carried. See
// printColumnLines.
const columnGap = 2

// lastInItsRow reports whether the word at index i is the last field its row
// has — the one field a row never pads.
//
// The columns are filled **down**, so the word after it in the same row is
// `rows` further along the list, and "there is no such word" is the whole of
// the test. A short last row therefore has its own last field, which is why
// `print -C 2 a b c` leaves `b` alone on the second line with nothing after
// it.
func lastInItsRow(i, rows, count int) bool { return i+rows >= count }

// rowTerminator is what ends each row of a column layout.
//
// Not [printOptions.terminator], which is the end of the whole text: a column
// layout writes one of these after **every** row including the last, and
// `-n` does not take it off. Measured — `print -n -C 2 a b c d` is two lines
// with both newlines, and `print -N -C 2 a b c d` is two rows each ended by a
// NUL and no newline at all.
func (o printOptions) rowTerminator() string {
	if o.nulSep {
		return "\x00"
	}
	return "\n"
}

// printColumnCount reads `-C`'s operand: how many columns, which must be a
// number and must be positive.
//
// Two refusals and they are worded differently, which is measured: a word
// that is not a number is `number expected after -C: x` and a number that is
// not positive is `invalid number of columns: 0`, both at status 1. The
// second covers negatives in the same words — `print -C -1 a b` is `invalid
// number of columns: -1`.
func printColumnCount(r *interp.Runner, arg string, opts *printOptions) int {
	n, err := strconv.Atoi(arg)
	if err != nil {
		r.Diagnosef("number expected after -C: %s\n", arg)
		return 1
	}
	if n <= 0 {
		r.Diagnosef("invalid number of columns: %d\n", n)
		return 1
	}
	opts.columns = n
	return -1
}

// columnDisplayWidth is how much room one field takes in the arithmetic: its
// display width with the control characters left out.
//
// See the note on printColumnLines for the four layouts that say so, and for
// why the table itself is the core's rather than this file's.
func columnDisplayWidth(s string) int {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return interp.DisplayColumns(b.String())
}
