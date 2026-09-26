// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A list's split edges are **fields of their own** where the word is
// distributed over the list, and boundaries where it is laid into it.
//
// The lay-in rule builds one word out of the whole list, so a boundary at
// either end has somewhere to go: it closes the field the text in front of
// the expansion is in and opens the one the text behind it starts. The
// distributive rule builds one copy of the word per field, and a boundary is
// not a field — so under it an element that split away to nothing produced no
// copy at all, and the copy real zsh makes for it went missing (#4581).
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// /opt/homebrew/bin/zsh under `-f`; `go version -m` on it says *not a Go
// executable*. `w(){ printf '%d' $#; printf '[%s]' "$@"; }`, every row under
// `setopt shwordsplit`, which is what makes an element split at all:
//
//	b=(' ' 2);     w x${^b}y   2[xy][x2y]        was 2[x][2y]
//	b=(' ');       w x${^b}y   2[xy][xy]         was 1[y]
//	b=(2 ' ');     w x${^b}y   2[x2y][xy]        was 2[x2][y]
//	b=(' a ' 2);   w x${^b}y   3[xy][xay][x2y]   was 3[x][ay][2y]
//	b=(' ' 2 ' '); w x${^b}y   3[xy][x2y][xy]    was 3[x][2][y]
//	b=(' ' 2);     w ${^b}y    2[y][2y]          was 1[2y]
//	b=(' ' 2);     w x${^b}    2[x][x2]          was 2[x][2]
//	b=(' ' 2);     w ${^b}     1[2]              already right
//
// **The rule is keyed on the *list* and not on the element**, which the last
// two of these say and the first four cannot: `(' ' 2 ' ')` is three copies,
// so the blank at each end made one each, while `(' ' ' ')` is **two** and
// not four — the two blanks are one leading edge and one closing one with a
// boundary between them that shows nothing. A rule stated about the element —
// "an element that splits to nothing still makes a copy" — answers that row
// four and agrees with every other row here.
//
// The last row is why the edge fields are marked as nulls rather than as
// ordinary empty fields: with nothing written beside the expansion the copy
// an edge made is empty and is removed, exactly as the field an empty element
// makes is. `x${^b}y` keeps it because the word wrote text into it. See
// interp/emptynullfield.go for the removal.
//
// The spelling is `${^spec}` or the same distribution under
// RC_EXPAND_PARAM, which are one code path and one row each above.

// spreadEdges is the parts and their null marks with the list's edges written
// in as fields, for the distributive rule.
//
// It answers the parts unchanged where there is no edge, which is every list
// that neither opens nor closes on a delimiter the split absorbed — and every
// caller that is not the unquoted list path, whose marks are empty.
func spreadEdges(parts []string, marks listMarks) ([]string, []bool) {
	if !marks.edges.lead && !marks.edges.openEnd {
		return parts, marks.nulls
	}
	out := make([]string, 0, len(parts)+2)
	nulls := make([]bool, 0, len(parts)+2)
	if marks.edges.lead {
		out = append(out, "")
		nulls = append(nulls, true)
	}
	for i, p := range parts {
		out = append(out, p)
		nulls = append(nulls, nullFieldAt(marks.nulls, i))
	}
	if marks.edges.openEnd {
		out = append(out, "")
		nulls = append(nulls, true)
	}
	return out, nulls
}
