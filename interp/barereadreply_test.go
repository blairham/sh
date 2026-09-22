// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// bareRead runs src with Semantics.BareReadTakesTheLineWhole under the test's
// control and the neighboring `read` questions answered flat.
func bareRead(t *testing.T, src string, whole interp.Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *interp.Runner) {
		sem := interp.CoreSemantics()
		sem.LastPipelineElementInCurrentShell = interp.Yes
		sem.TrailingSeparatorEndsAField = interp.No
		sem.ReadRequiresAVariableName = interp.No
		sem.BareReadTakesTheLineWhole = whole
		r.Semantics = &sem
	})
}

// A `read` with no names fills the shell's own, and the panel splits over
// whether that name is an operand like any other — so trimmed the way an
// operand's value is — or the record as it came.
func TestABareReadAsksWhatItsDefaultNameHolds(t *testing.T) {
	for _, tc := range []struct{ name, src, trimmed, whole string }{
		{
			"whitespace at both ends",
			`printf '  A B  \n' | { read; printf '[%s]' "$REPLY"; }`,
			`[A B]`, `[  A B  ]`,
		},
		{
			"a leading run only",
			`printf '\tA B\n' | { read; printf '[%s]' "$REPLY"; }`,
			`[A B]`, "[\tA B]",
		},
		{
			// The inner run is never touched under either answer: this is
			// the trim at the ends and not a re-join of the fields.
			"whitespace inside the record",
			`printf ' a  b \n' | { read; printf '[%s]' "$REPLY"; }`,
			`[a  b]`, `[ a  b ]`,
		},
		{
			// The escapes are processed either way, so the backslash-space
			// is a space in both columns and only the closing run moves.
			"an escaped space and a trailing one",
			`printf 'a\\ b \n' | { read; printf '[%s]' "$REPLY"; }`,
			`[a b]`, `[a b ]`,
		},
		{
			// -r keeps the backslash, and still only the trim moves.
			"the same line under -r",
			`printf 'a\\ b \n' | { read -r; printf '[%s]' "$REPLY"; }`,
			`[a\ b]`, `[a\ b ]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := bareRead(t, tc.src, interp.No); out != tc.trimmed || st != 0 {
				t.Errorf("an operand like any other: got %q status %d, want %q at 0", out, st, tc.trimmed)
			}
			if out, st := bareRead(t, tc.src, interp.Yes); out != tc.whole || st != 0 {
				t.Errorf("the record whole: got %q status %d, want %q at 0", out, st, tc.whole)
			}
		})
	}
}

// The guard, and the half two answered runs cannot show: the question is put
// only where the trim would take something off, so `while read; do` under an
// unanswered vector runs without a word.
func TestABareReadOnAnOrdinaryRecordAsksNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"nothing to trim",
			`printf 'A B\n' | { read; printf '[%s]' "$REPLY"; }`,
			`[A B]`,
		},
		{
			"an empty record",
			`printf '\n' | { read; printf '[%s]' "$REPLY"; }`,
			`[]`,
		},
		{
			// A non-whitespace IFS makes the spaces data, so neither
			// reading takes them off and there is nothing to decide.
			"spaces that are not separators",
			`IFS=:; printf '  a:b  \n' | { read; printf '[%s]' "$REPLY"; }`,
			`[  a:b  ]`,
		},
		{
			// A named operand is not the default name, so the question
			// belongs to no line with one on it however the record ends.
			"a name the script wrote",
			`printf '  A B  \n' | { read x; printf '[%s]' "$x"; }`,
			`[A B]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := bareRead(t, tc.src, interp.Unspecified)
			if out != tc.want || st != 0 {
				t.Errorf("unanswered: got %q status %d, want %q at 0 with nothing said", out, st, tc.want)
			}
		})
	}
}
