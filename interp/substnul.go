// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// substitutedValue is what a command substitution's captured output becomes:
// the trailing newlines removed, and a NUL byte in it given the dialect's
// answer. Every spelling that captures output comes through here — `$( … )`,
// the backquoted form, `$(<file)` and `${ … ;}` — so the rule has one home.
// See Semantics.NulInAValue for the panel.
//
// The order is measured, and it differs by answer. Where the byte is dropped
// it goes first and the newlines after: `$(printf 'a\n\0')` is `a` in dash,
// bash 5.3.20 and BusyBox ash 1.37.0, so a newline that only a NUL kept away
// from the end is trailing once the NUL is gone. Where the byte stays — kept,
// or held for the cut — the newlines go and the NUL is still there to stop
// them: the same line is `a` and a newline in ksh93u+, which cuts what is
// left at the NUL afterwards (see Runner.cutAtNul).
//
// Asked only where a NUL actually arrived, so every ordinary substitution
// asks nothing. An unanswered vector refuses by name, at status 2, with an
// empty value — the read builtin's refusal for the same question.
func (r *Runner) substitutedValue(out string) string {
	if strings.IndexByte(out, 0) >= 0 {
		switch r.sem().NulInAValue {
		case NulKept, NulCutInASubstitution:
		case NulDropped:
			out = strings.ReplaceAll(out, "\x00", "")
			// Said once for the substitution, however many there were, in
			// the one dialect that says anything. See
			// Diagnostics.SubstitutionDroppedANul.
			if w := r.diag().SubstitutionDroppedANul; w != "" {
				r.diagf("%s\n", w)
			}
		default:
			r.diagf("%s\n", r.unanswered("a NUL byte in what a command substitution takes in"))
			r.status = 2
			r.unspecified = true
			return ""
		}
	}
	return strings.TrimRight(out, "\n")
}

// cutAtNul ends a finished field at the first NUL it holds, in the dialect
// that reads a substituted NUL that way. See Semantics.NulInAValue.
//
// The cut is the *field's*, not the substitution's, and that is measured
// rather than a convenience: in ksh93u+ `"x$(printf 'a\0b')y"` is `xa` — the
// `y` written after the substitution is gone too — while `$(printf 'a\0b c')`
// unquoted is the two fields `a` and `c`, so a separator after the NUL still
// makes a field of its own. A value cut inside the substitution could answer
// neither: it would keep the `y` and lose the `c`. Holding the byte until the
// word is finished and then ending each field at it answers both.
//
// Exact rather than approximate in that dialect because nothing else there
// puts a NUL in a field: `read` drops one, a `$'\0'` ends its own span, and a
// variable cannot be given one by any other route. So the byte a field holds
// at this point came out of a substitution. The other answers never reach the
// cut — dropped values hold no NUL, and kept ones are meant to keep it.
func (r *Runner) cutAtNul(field string, escaped bool) string {
	if strings.IndexByte(field, 0) < 0 || r.sem().NulInAValue != NulCutInASubstitution {
		return field
	}
	if !escaped {
		return field[:strings.IndexByte(field, 0)]
	}
	// The escaped form has two kinds of NUL in it, and only one is data. A
	// bare one is valueBackslashMark, which escapeValueBackslashes writes for
	// a backslash a value held; a data NUL is always written behind the
	// backslash that marks it as content (markedByGlobEscape holds the byte).
	// So the field is read left to right a mark at a time, and the cut is at
	// the first marked NUL — taking its mark with it, since a lone escape left
	// at the end of the field would unescape to a backslash nobody wrote.
	for i := 0; i < len(field); i++ {
		if field[i] != '\\' || i+1 >= len(field) {
			continue
		}
		if field[i+1] == 0 {
			return field[:i]
		}
		i++
	}
	return field
}

// cutFieldsAtNul is cutAtNul over a word's fields, in place. The fields are
// in the escaped form, which is what every caller hands it.
func (r *Runner) cutFieldsAtNul(fields []string) []string {
	for i, f := range fields {
		fields[i] = r.cutAtNul(f, true)
	}
	return fields
}
