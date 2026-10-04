// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// substBodyKey names one substitution's text where it was written: two
// spellings of the same text at the same place are one body.
type substBodyKey struct {
	at   syntax.Pos
	text string
}

// keptSubstBody is a body parsed with the line that holds it.
type keptSubstBody struct {
	f    *syntax.File
	base int
}

// keepSubstBody keeps the tree a body was read into with its line, in the
// dialect that runs that tree rather than reading the text again when the
// substitution runs. See Semantics.SubstitutionRunsTheBodyReadWithItsLine.
func (r *Runner) keepSubstBody(span syntax.Span, f *syntax.File, base int) {
	if !r.ask(r.sem().SubstitutionRunsTheBodyReadWithItsLine,
		"a substitution running the body read with its line") {
		return
	}
	if r.keptSubstBodies == nil {
		r.keptSubstBodies = map[substBodyKey]keptSubstBody{}
	}
	r.keptSubstBodies[substBodyKey{span.Pos, span.Value}] = keptSubstBody{f, base}
}

// keptSubstBody answers the tree keepSubstBody kept for a span, if any.
func (r *Runner) keptSubstBody(span syntax.Span) (*syntax.File, int, bool) {
	k, ok := r.keptSubstBodies[substBodyKey{span.Pos, span.Value}]
	return k.f, k.base, ok
}
