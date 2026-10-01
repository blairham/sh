// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// jobSpecQuestionMark makes the `?` of a field that begins `%?` a character
// rather than a pattern, in the dialect that reads that spelling as the
// `%?string` job spec. See Semantics.JobSpecQuestionMarkIsLiteral.
//
// The field is in the escaped form, so a `?` that is already content arrives
// behind its backslash and is left alone, and a `%` that was quoted arrives
// bare — `%` is no metacharacter, so quoting it leaves no mark — which is
// what makes `'%'?` the same word as `%?` here, as it is in the reference.
//
// Asked only where the field really begins with `%` and a live `?`, which is
// the one place the columns part.
func (r *Runner) jobSpecQuestionMark(field string) string {
	if len(field) < 2 || field[0] != '%' || field[1] != '?' {
		return field
	}
	if !r.ask(r.sem().JobSpecQuestionMarkIsLiteral,
		"a `?` right after a word's leading `%` being a character rather than a pattern") {
		return field
	}
	return "%\\?" + field[2:]
}
