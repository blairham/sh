// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"

	"github.com/blairham/sh/syntax"
)

// An assignment's value made of plain text, bare reads and their lengths.
//
// `x=0`, `x=${t}lit`, `x=${#v}`: values a script writes all the time, and each
// went through the whole of expandAssignValue's road — a copy of the word for
// the tilde passes, the leading-tilde and colon-tilde passes, the `=cmd` pass,
// and then the span loop under a glob guard — to come back as the text the
// spans already held. That road was most of what the three cost, about 3x zsh
// on each in the F-Sy-H highlighter's profile (#6423).
//
// A span is admitted only where none of those passes could act on it:
//
//   - an unquoted literal of plainField's alphabet, which has no `~` for a
//     tilde pass, no `=` for the equals pass, and no quote, escape, glob or
//     brace character for the span loop to remove or keep;
//   - a bare `$name` or `${name}` that plainParamSpan reads, which is an
//     ordinary scalar's text and nothing else;
//   - `${#name}` of the same kind of name, set, which is stringLength — the
//     full road's own count, so a multibyte value is counted exactly as there.
//
// Anything else sends the whole value to the full road having read nothing
// that matters: a read of an ordinary scalar changes nothing, which is the
// arrangement plainParamValue already rests on.
// TestAPlainAssignValueAnswersAsTheFullRoadDoes runs scripts over each shape
// and its neighbors with the route on and off and requires the same output.

// plainAssignValueShortcut switches plainAssignValue on. A variable so a test
// can run the same script with the full road answering every value.
var plainAssignValueShortcut = true

// plainAssignValue is the text of an assignment's value whose every span is
// one of the three shapes above.
func (r *Runner) plainAssignValue(w *syntax.Word) (string, bool) {
	if !plainAssignValueShortcut || w == nil || len(w.Spans) == 0 || !r.readsAreOnlyReads() {
		return "", false
	}
	var v string
	for i := range w.Spans {
		t, ok := r.plainAssignSpan(&w.Spans[i])
		if !ok {
			return "", false
		}
		v += t
	}
	return v, true
}

// plainAssignSpan is one span's text, where it is one plainAssignValue admits.
func (r *Runner) plainAssignSpan(s *syntax.Span) (string, bool) {
	if s.Quoting != syntax.Unquoted {
		return "", false
	}
	switch s.Kind {
	case syntax.Literal:
		if s.Value == "[" || !plainField(s.Value) {
			// A lone `[` is a field plainField admits as the test builtin's
			// name, which says nothing about a value.
			return "", false
		}
		return s.Value, true
	case syntax.ParamExp:
		if s.Param != nil && s.Param.Length {
			return r.plainLengthOf(s.Param)
		}
		return r.plainParamSpan(s, false)
	}
	return "", false
}

// plainLengthOf is `${#name}` of an ordinary scalar that is set, counted by the
// full road's own stringLength.
func (r *Runner) plainLengthOf(e *syntax.ParamExpr) (string, bool) {
	if !plainParamShortcut {
		return "", false
	}
	// The expansion with its length mark taken off must be a bare name: a
	// copy, because the tree is the program's and may be read elsewhere.
	bare := *e
	bare.Length = false
	if !bareParamOnly(&bare) || !identifierName(e.Name) || !r.ordinaryScalarName(e.Name) {
		return "", false
	}
	v, set := r.getVar(e.Name)
	if !set {
		// `set -u` is answered on the full road, and so is what an unset
		// name's length is in each dialect.
		return "", false
	}
	return strconv.Itoa(r.stringLength(v)), true
}
