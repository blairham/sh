// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

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
// the fields two ways. An array target takes them as fields, where this
// question is live at every count. A list of names takes the last one as the
// remainder of the *line* — text rather than a field — and there the extra
// field changes a value only when it carries the count from one field per name
// to one more than there are names: `IFS=: read x y` on `a:b:` is `b` where
// the separator is absorbed and `b:` where it opens a field. So `read`'s names
// call this at that count and not otherwise, which is the caller's guard and
// not one this function can put.
//
// Two guards here, and both are what keeps the axis off the ordinary script
// rather than an optimization. It is asked only where the two readings differ — a
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

// The two questions `read` asks on top of that, both of which only its own
// splitting can see. See Semantics.ReadTrailingWhitespaceEndsAField and
// Semantics.ReadNoFieldsIsOneEmptyElement for the measurements.

// readFieldsTail is trailingSeparatorField with `read`'s extra question:
// whether a closing run of IFS *whitespace* opens a field of its own as well.
//
// It is `read`'s and not the splitter's because the splitter's answer was
// measured on an expansion and is the other way: zsh absorbs trailing
// whitespace in `${=v}` — `x=" a "` is one field — and opens a field for it in
// `read`. One shell, two answers, same IFS, so it cannot be one field.
//
// Asked only where the closing run is whitespace *and* holds no non-whitespace
// separator, because the run with one in it has already been decided above and
// asking again would add a second empty field.
func (r *Runner) readFieldsTail(fields []string, s string, literal []bool, ifs string, ifsSet bool) []string {
	if out := r.trailingSeparatorField(fields, s, literal, ifs, ifsSet, false); len(out) != len(fields) {
		return out
	}
	if r.unspecified || !trailingRunIsWhitespace(s, literal, ifs, ifsSet) {
		return fields
	}
	if !r.ask(r.sem().ReadTrailingWhitespaceEndsAField,
		"a closing run of IFS whitespace opening a field of its own in `read`") {
		return fields
	}
	return append(fields, "")
}

// trailingRunIsWhitespace reports whether the value ends in an unescaped run
// of IFS whitespace and nothing else.
//
// The mirror of trailingRunSeparates: that one answers whether the closing run
// holds a separator the panel disagrees about, and this one whether the run is
// there at all with only whitespace in it. An escaped separator is data and
// ends the run, the same rule the other follows, so `read -A` on `a\ ` is one
// field in every reading.
// No guard for an IFS set to nothing and none for an IFS that is unset, and
// both are provable rather than overlooked: with `IFS=` the membership test at
// the end can never succeed, and Runner.ifs already hands an unset IFS back as
// the default three characters. Either as a line of its own was a branch no
// mutation could kill.
func trailingRunIsWhitespace(w string, literal []bool, ifs string, ifsSet bool) bool {
	i := len(w) - 1
	if i < 0 || (literal != nil && literal[i]) {
		return false
	}
	c := w[i]
	if c != ' ' && c != '\t' && c != '\n' {
		return false
	}
	return strings.IndexByte(ifs, c) >= 0
}
