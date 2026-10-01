// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A backslash written into `IFS` separates, and the escaped form is what hid
// it.
//
// A backslash that arrived in a **value** is not written as a backslash: it
// stands in the escaped form as valueBackslashMark, or as that mark doubled
// where the value ran out behind it, because the form spells "this byte was
// quoted" as a backslash and a value's own backslash would be read as that
// mark. See interp/glob.go, which is where the two spellings are argued.
//
// The splitter reads the form byte by byte and tested `s[i]` against `IFS`,
// so the byte it tested at such a position was the mark rather than the
// backslash — and the character never reached the separator test at all. The
// comment on splitFieldsAt's cutAt says the opposite is intended: *a marked
// separator still separates — a value's backslash quotes for the match and
// never for the split.* It was true of a backslash that was quoted and false
// of a backslash that was the value (#4585).
//
// Measured 2026-09-26 with
// `w(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; }`
// against bash 5.3.20 at /opt/homebrew/bin/bash, ksh93u+ 2012-08-01 at
// /bin/ksh, dash 0.5.12 at /bin/dash and zsh 5.9.2 at /opt/homebrew/bin/zsh
// under `shwordsplit`. All four agree on every row, and this shell answered
// the right-hand column:
//
//	IFS='\'; v='b\c';  w x$v y    3  [xb] [c] [y]     was 2  [xb\c] [y]
//	IFS='\'; v='\';    w x${v}y   2  [x] [y]          was 1  [x\y]
//	IFS='\'; v='b\';   w x${v}y   2  [xb] [y]         was 1  [xb\y]
//	IFS='\'; v='\c';   w x${v}y   2  [x] [cy]         was 1  [x\cy]
//	IFS='\'; v='b\\c'; w x${v}y   3  [xb] [] [cy]     was 2  [xb\] [cy]
//	IFS='\:'; v='b\c:d'; w x${v}y 3  [xb] [c] [dy]    was 2  [xb\c] [dy]
//	IFS='\'; v='b\c';  w ${v}     2  [b] [c]          was 1  [b\c]
//
// The fifth row is the one that says the mark is the whole of it: a value
// holding **two** backslashes already split on the second, because that one
// is written as an ordinary marked backslash and the walk could see it. One
// of the two characters separated and the other did not, from the same value
// and the same `IFS`.
//
// It is **core**, with no axis: the panel is unanimous, and the two
// non-whitespace separators of the fifth row produce one empty field between
// them under the ordinary delimiter rule rather than under anything of this
// file's.

// valueBackslashSeparator reports how many bytes of the escaped form a value's
// backslash takes at i, when a backslash is a separator in ifs. It answers 0
// where the position is not one of the two marks, or where `IFS` holds no
// backslash.
//
// The width is the form's and not the value's: the mark is one byte in front
// of an ordinary escape, and the doubled mark is two bytes standing for one
// backslash that ran out of value. Returning the form's width is what lets
// the caller step past the separator without reading half of a mark as data.
//
// A bare mark can only be one the escaping wrote, which is what makes the
// test a byte comparison rather than a search — but only in a string that was
// escaped at all, and only where no escape stands in front of the NUL. The
// caller decides both before asking: see dataNUL in splitFieldsAt, which was
// added when a zsh value's NUL turned out to be read as this mark (#5263).
// isMark alone never refused it, since the escape is the byte before the NUL
// and not the NUL itself. See valueBackslashMark.
func valueBackslashSeparator(s string, i int, ifs string) int {
	if i >= len(s) || s[i] != valueBackslashMark {
		return 0
	}
	if strings.IndexByte(ifs, '\\') < 0 {
		return 0
	}
	if strings.HasPrefix(s[i:], valueBackslashRanOutOfValue) {
		return len(valueBackslashRanOutOfValue)
	}
	return 1
}
