// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// An empty element of an unquoted list is a *field*, not nothing.
//
// Every shell in the panel drops it from the values — `a=('' 2); $a` is the
// one word `2` — and every shell in the panel leaves the **field boundary**
// behind, so text written beside the expansion does not join across the gap.
// Measured 2026-09-25 against zsh 5.9.2, bash 5.3.20, ksh93u+ and dash, with
// `w(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; }`:
//
//	set -- '' 2;      w x$@y   2  [x] [2y]     the empty element closed `x`
//	set -- 1 '';      w x$@y   2  [x1] [y]     and opened `y` at the other end
//	set -- '' '';     w x$@y   2  [x] [y]      one at each end, two fields
//	set -- '';        w x$@y   1  [xy]         **one** element takes both
//	set -- '' 2 '';   w x$@y   3  [x] [2] [y]
//	set -- '' '' 2;   w x$@y   2  [x] [2y]     an interior one shows nothing
//	set -- '' 2;      w $@     1  [2]          nothing beside it, nothing to keep
//	set -- '' 2;      w $@y    1  [2y]
//	set -- '' 2;      w x$@    2  [x] [2]
//
// The rule the rows are of is a sentence with one noun in it: **the field an
// empty element makes is removed only when nothing in the word joined text to
// it.** The noun is the *field*, not the element — which is why `set -- ''`
// is one word and `set -- '' ''` is two. One element is one field, and one
// field absorbs the text on both sides of the expansion at once; two elements
// are two fields, and the second one is where the `y` goes.
//
// Reading it as the element is what this shell did, and the two readings
// agree on every row that has no text beside the expansion — which is most
// rows, and is why a grid of array references reports it clean. They also
// agree wherever the empty element is in the *middle*: the fields on either
// side of it are separate whichever way the null is removed. Holding the
// element fixed and moving the text is what parts them.
//
// So the drop moved from the element to the word: splitEachElement keeps the
// empty element as an empty field and marks it, wordFields lays the marked
// fields in with every other, and the mark is spent the moment anything else
// reaches that field. What is left marked and empty at the end is what
// wordFields.result removes.
//
// Quoting is not this question. `"${a[@]}"` keeps every field including the
// empty one in every column, and it always did here — the marks are only ever
// set on an *unquoted* list, which is the one reading the removal is about.

// nullFieldAt reports whether the field at i is one of the empty elements an
// unquoted list held.
//
// Indexed rather than ranged over because the marks travel beside the fields
// through stages that may not have produced them — a nil slice is "nothing
// was marked", which is what every caller outside the unquoted list path
// hands in.
func nullFieldAt(nulls []bool, i int) bool {
	return i < len(nulls) && nulls[i]
}

// withoutNullFields is the fields with the marked empty elements taken out,
// which is what this pipeline answered everywhere before the word builder
// learned to keep them.
//
// It is how a caller that is not building a command line's words opts out: a
// redirection target is one name rather than a list, and the axis that
// decides between the two readings of an unquoted list is asked on the fields
// the old reading produced, so that adding the marks moved no dialect's
// answer.
func withoutNullFields(parts []string, nulls []bool) []string {
	if !slices.Contains(nulls, true) {
		return parts
	}
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		if nullFieldAt(nulls, i) {
			continue
		}
		out = append(out, p)
	}
	return out
}
