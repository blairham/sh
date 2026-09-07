// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The tail of the field-splitting rule, which is the one part of it the panel
// does not agree on. See Semantics.TrailingSeparatorEndsAField for the
// measurements; this file is where the answer is asked and applied.
//
// It sits beside the splitter rather than inside it because the splitter is a
// pure function four call sites share and this is a question only a Runner can
// put. What the answer does to a split is exactly one empty field at the end
// and nothing else, so the pure rule stays whole and this is a step after it.

// splitFieldsAsking is the field-splitting stage with that question asked:
// splitFieldsEdges, and then the field a trailing non-whitespace separator
// opens where the dialect says it opens one.
func (r *Runner) splitFieldsAsking(s string, literal []bool, ifs string, ifsSet, keepEdges bool) []string {
	out := splitFieldsEdges(s, literal, ifs, ifsSet, keepEdges)
	return r.trailingSeparatorField(out, s, literal, ifs, ifsSet, keepEdges)
}

// splitFieldsAsk is splitFieldsAsking for the callers that have no escape
// mask and no edge-keeping rule in force, which is every expansion but
// `${=spec}`.
func (r *Runner) splitFieldsAsk(s, ifs string, ifsSet bool) []string {
	return r.splitFieldsAsking(s, nil, ifs, ifsSet, false)
}

// trailingSeparatorField adds the field the closing separator opens, to a
// split that has already happened.
//
// It is separate from splitFieldsAsking for `read`, which splits once and uses
// the fields two ways: an array target takes them as fields, where this
// question is live, and a list of names takes the last one as the remainder of
// the line, which is text rather than a field and is not this question.
//
// Two guards, and both are what keeps the axis off the ordinary script rather
// than an optimization. It is asked only where the two readings differ — a
// closing run of separators holding a non-whitespace one — so a whitespace IFS
// never reaches it and neither does a value ending in anything else. And under
// the edge-keeping rule of a quoted `${=spec}` the field behind the last
// separator is there already, so there is nothing to decide and asking would
// add a second one.
func (r *Runner) trailingSeparatorField(fields []string, s string, literal []bool, ifs string, ifsSet, keepEdges bool) []string {
	if keepEdges || !trailingRunSeparates(s, literal, ifs, ifsSet) {
		return fields
	}
	if !r.ask(r.sem().TrailingSeparatorEndsAField,
		"a trailing IFS separator opening a field of its own") {
		return fields
	}
	return append(fields, "")
}
