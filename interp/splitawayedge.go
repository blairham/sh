// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// An element that splits away to nothing still leaves its field boundary.
//
// #4560 was the element whose *value* is empty, and the mark it added says
// which field an element made. This is the element whose value is nothing but
// separators, which makes **no field to mark** — and the boundary it leaves
// is the one the scalar path has recorded since #3373, read off the split
// rather than off the fields:
//
//	leadingSeparatorEdge   the value opens on IFS whitespace, so the field in
//	                       front of the expansion is finished
//	splitFieldsAskEdge     openEnd: the value closed on a delimiter the split
//	                       absorbed, so the field behind it is opened by
//	                       whatever the word writes next
//
// `v=" "; x${v}y` is `[x] [y]` here and has been since #3373. The list path
// made neither record, because splitEachElement splits each element in
// isolation and hands back only the fields, so an element that produced none
// left nothing behind at all. Measured 2026-09-26 with
// `w(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; }`
// against bash 5.3.20, ksh93u+ 2012, dash 0.5.12, BusyBox ash 1.37.0 and zsh
// 5.9.2 under `shwordsplit` — the five columns that split a list — all five
// agreeing on every row:
//
//	set -- ' ' 2;     w x$@y   2  [x] [2y]     the blank element closed `x`
//	set -- 2 ' ';     w x$@y   2  [x2] [y]     and opened `y` at the far end
//	set -- ' ';       w x$@y   2  [x] [y]      one element, both edges
//	set -- ' ' 2 ' '; w x$@y   3  [x] [2] [y]
//	set -- ' ' ' ' 2; w x$@y   2  [x] [2y]     a run of them is one boundary
//	set -- ' a ' 2;   w x$@y   3  [x] [a] [2y] an element that splits to one
//	set -- ' ';       w $@y    1  [y]          nothing in front, nothing kept
//	set -- ' ';       w x$@    1  [x]          nothing behind, nothing opened
//	IFS=:; set -- ':'; w x$@y  2  [x] [y]      the non-whitespace spelling
//
// **The rule is a sentence with one noun in it: the *list* records the edges
// of its split, not the element.** The noun is the list because only the
// first element's opening edge and the last element's closing one can ever
// show: an element in the middle is already a field boundary away from its
// neighbors, so its own edges have nothing to separate. The pair that says
// so holds the element fixed and moves it: `set -- ' ' 2 ' '` is three fields
// where `set -- 2 ' ' 3` is two, the same blank element reading as a boundary
// at an end and as nothing in the middle.
//
// It is **core**, with no axis: every row above is unanimous across all five
// splitting columns, and the sixth — zsh with its splitting off — is not this
// question, since there a blank element is a literal and stays one.
//
// A field the splitter made is still not a field an element made, which #4560
// established and this must not disturb: `IFS=:; set -- ':b' c; $@` is
// `[][b][c]`, the leading null kept because the splitter wrote it. The edges
// are a *boundary* and never a field, so they add nothing to remove.

// listEdges is what the split of an unquoted list left at its two ends, which
// is the scalar path's pair of records taken over the list.
//
// lead is the first element's leading IFS whitespace, openEnd the last
// element's closing delimiter where the split absorbed it without writing a
// field. Both are boundaries with nothing to show for themselves, and both
// are read off the split rather than off the fields — which is why they have
// to travel beside the fields instead of being recoverable from them.
type listEdges struct {
	lead    bool
	openEnd bool
}

// listMarks is everything the unquoted list path leaves beside its fields:
// the per-field marks #4560 added and the per-list edges above.
//
// One record rather than two traveling in parallel. They are set in the same
// place, cleared in the same place and read in the same place, and a second
// channel carrying half of them is how the next stage comes to get one and
// not the other. The Runner holds the two halves as separate fields for a
// reason of its own — see Runner.listEdges — and joins them here.
type listMarks struct {
	// nulls is, per field, whether that field is one of the empty *elements*
	// the list held rather than anything the splitter made. See
	// interp/emptynullfield.go.
	nulls []bool
	// edges is the boundary the split left at either end of the list.
	edges listEdges
}
