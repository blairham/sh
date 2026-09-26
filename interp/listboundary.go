// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"
)

// The boundary between two adjacent elements of an unquoted list, and the one
// question the panel has two answers to about it.
//
// #4560 was the field an empty element makes and #4573 the boundary an element
// that splits away to nothing leaves; both are about what an *element* comes
// to, and both are core. This is about the gap between two of them, and it is
// not core: dash and BusyBox ash read that gap as a delimiter of the ordinary
// splitting rule — IFS whitespace — where bash, ksh93 and zsh read it as a
// field break the split cannot see past.
//
//	IFS=:; set -- b ':'; w x$@y
//
// is `[xb] [] [y]` in bash 5.3.20, ksh93u+ and zsh under `shwordsplit`, and
// `[xb] [y]` in dash 0.5.12 and BusyBox ash 1.37.0 — the separator written in
// the second element joining the boundary in front of it rather than writing a
// field between them. See Semantics.UnquotedListBoundaryIsIFSWhitespace for
// the grid, the three readings it rules out and where each was measured.
//
// The reading is implemented by *giving the splitter the boundaries*, rather
// than by splitting each element and merging the results afterwards. A merge
// would be a second statement of "one delimiter is a run of whitespace, at
// most one separator, a run of whitespace", and a second statement of that
// rule is a second place for it to drift — which is the reason splitFieldsAt
// reports its own offsets and its own open end instead of letting a caller
// walk the string again.
//
// Two things follow from doing it that way, and both are load-bearing:
//
//   - The boundary is spelled as a byte in the joined string and marked in a
//     mask beside it, so which byte it is never matters. A sentinel *value*
//     would have to be a byte no value can hold, and the escaped form has
//     already spent the only one there is — see valueBackslashMark.
//   - The edges the word reads afterwards are the *joined* string's, which is
//     what makes a list that opens or closes on an empty element close the
//     field beside it: `IFS=:; set -- '' 2; x$@y` is `[x] [2y]`, where the
//     boundary is the leading delimiter and there is no field for it.

// listBoundaryByte stands where a boundary stands in the joined string. It is
// a space because a space is legible in a debugger; the mask beside it is what
// makes it a delimiter, so any byte would do and none is reserved.
const listBoundaryByte = ' '

// listBoundaryFields is the reading that gives the split the boundaries, and
// whether this dialect takes it.
//
// ok false means the caller keeps the reading it already has: either the two
// readings coincide here, or the dialect answered that a boundary is a field
// break. Nothing is asked where the answer could not change a field, which for
// this question is boundaryCouldSplitDifferently below.
func (r *Runner) listBoundaryFields(elems, perElement []string, marks listMarks,
	sp splitPolicy, glob Answer,
) ([]string, listMarks, bool) {
	if !r.boundaryCouldSplitDifferently(elems) {
		return nil, listMarks{}, false
	}
	// The splitting answer stands in front of this one, exactly as it does in
	// front of the join: with splitting off there is no delimiter rule for a
	// boundary to be part of, and the elements are one field each in every
	// column.
	if !r.ask(sp.answer(r.sem().SplitParamExpansion),
		"splitting an unquoted parameter expansion") {
		return nil, listMarks{}, false
	}
	across, edges := r.splitAcrossBoundaries(elems, glob)
	perFields, perEdges := listShape(perElement, marks)
	if slices.Equal(perFields, across) && perEdges == edges {
		return nil, listMarks{}, false
	}
	if !r.ask(r.sem().UnquotedListBoundaryIsIFSWhitespace,
		"the boundary between two elements of an unquoted list being an IFS delimiter") {
		return nil, listMarks{}, false
	}
	// No null marks: what the splitter made of the joined string is an
	// ordinary field, empty ones included. The elements went into it and
	// there is nothing left that is an empty *element* — the same thing the
	// join says about its own result, for the same reason.
	return across, listMarks{edges: edges}, true
}

// boundaryCouldSplitDifferently reports whether the two readings could give
// different fields for these elements.
//
// They can differ only where a delimiter run reaches across a boundary, which
// needs a separator at the edge of an element beside one. Two elements at
// least, then, and one of them with a character of IFS at its front or its
// back:
//
//   - `set -- a b` is `[xa] [by]` either way — a boundary on its own is a
//     delimiter under both readings, one because it is whitespace and the
//     other because it is a break.
//   - An IFS that is set and empty splits nothing, so its elements are one
//     field each whatever this says — and the test below cannot succeed on
//     it, which is why there is no line for it here.
//   - An *empty* element is not enough on its own. It makes no field under
//     the boundary reading and a removable null under the other, and the two
//     agree on the word: `set -- b ” c; x$@y` is `[xb] [cy]` in both.
func (r *Runner) boundaryCouldSplitDifferently(elems []string) bool {
	if len(elems) < 2 {
		return false
	}
	ifs, _ := r.ifs()
	for _, el := range elems {
		if el == "" {
			continue
		}
		if strings.IndexByte(ifs, el[0]) >= 0 || strings.IndexByte(ifs, el[len(el)-1]) >= 0 {
			return true
		}
	}
	return false
}

// splitAcrossBoundaries splits the whole list at once, with the boundary
// between each pair of elements standing in the string as a delimiter.
//
// The escape runs per element and before the split, as the per-element reading
// does it and for the same reason: what it adds is backslashes, and no IFS
// puts a field boundary on one.
func (r *Runner) splitAcrossBoundaries(elems []string, glob Answer) ([]string, listEdges) {
	ifs, set := r.ifs()
	var b strings.Builder
	boundary := make([]bool, 0, len(elems))
	for i, el := range elems {
		if i > 0 {
			b.WriteByte(listBoundaryByte)
			boundary = append(boundary, true)
		}
		esc := r.escapeResult(el, glob)
		b.WriteString(esc)
		for range len(esc) {
			boundary = append(boundary, false)
		}
	}
	s := b.String()
	// The leading edge is the joined string's, read by the one function that
	// knows the opening-run rule and handed the mask so that a boundary
	// counts as whitespace in it. `IFS=:; set -- '' 'b:'; x$@y` is `[x] [b]
	// [y]` — the boundary an empty first element leaves is the run, and there
	// is no field for it — while `IFS=:; set -- '' ':' b; x$@y` is `[x] [by]`,
	// where the run holds a separator whose own empty field already joins the
	// `x`.
	lead := leadingSeparatorEdge(s, boundary, ifs, set)
	fields, openEnd := r.splitFieldsAskEdge(s, boundary, ifs, set)
	return fields, listEdges{lead: lead, openEnd: openEnd}
}

// listShape reduces a reading to the form the word is actually built from, so
// that two readings which write the same word compare equal.
//
// The edges and the null *elements* are two spellings of one thing and the two
// readings do not use the same one: a boundary reading has no element left to
// be empty and says the same thing with an edge, where the per-element reading
// has the element and no edge. Compared as they stand, `IFS=:; set -- 'b:' ”;
// x$@y` looks like a disagreement and is `[xb] [y]` under both — and the axis
// would be put in front of a word no shell reads two ways.
//
// The reduction is the removal rule of interp/emptynullfield.go read
// structurally rather than applied: a null element at the front of the fields
// closes whatever text stood in front of the expansion, which is what lead
// says; one at the back opens whatever follows, which is openEnd; and one in
// the middle has a field on each side already and leaves nothing behind. What
// it is **not** is a rule about a lone null element taking the text on both
// sides at once — `set -- ”; x$@y` is the single word `xy` — and that shape
// cannot arrive here, because a list of one element has no boundary and
// boundaryCouldSplitDifferently has already refused it.
func listShape(fields []string, marks listMarks) ([]string, listEdges) {
	edges := marks.edges
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		if i < len(marks.nulls) && marks.nulls[i] {
			if i == 0 {
				edges.lead = true
			}
			if i == len(fields)-1 {
				edges.openEnd = true
			}
			continue
		}
		out = append(out, f)
	}
	return out, edges
}
