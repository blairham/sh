// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// **A subscript does not aim a reference.**
//
// A write to a reference with nothing to point at is the write that aims it —
// `typeset -n ref; ref=foo` leaves `declare -n ref="foo"`. A *subscripted*
// write is not that write: it names an element of whatever the reference points
// at, and it points at nothing, so the element has no name to belong to. This
// shell threw the subscript away and let the value aim the reference, so `ref`
// came out pointing at the value and the element was written nowhere.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	typeset -n ref; ref[0]=foo    `` `': not a valid identifier ``, 1, and
//	                              `ref` still `declare -n ref`
//	typeset -n ref; ref=foo       taken, and the reference is aimed
//
// The empty name in the sentence is the reference's target standing where the
// element's base belongs, which is why the word it quotes is empty whatever the
// subscript was (#4178).
func TestASubscriptDoesNotAimAReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref12.sub` grades.
			"a subscripted write over an unaimed reference is refused",
			"typeset -n ref\nref[0]=foo\nprintf '<%d>' $?",
			"<1>", "",
		},
		{
			"and the reference is left unaimed",
			"typeset -n ref\nref[0]=foo\ntypeset -p ref",
			"declare -n ref", `declare -n ref="foo"`,
		},
		{
			// The word quoted is empty whatever the subscript was, because
			// it is the reference's target and not the subscript.
			"a key subscript is refused the same way",
			"typeset -n ref\nref[k]=foo\ntypeset -p ref",
			"`': not a valid identifier", `ref="foo"`,
		},
		{
			// The cost is the bare assignment's: the rest of the line goes
			// and the next line runs.
			"the rest of the line goes and the next line runs",
			"typeset -n ref\nref[0]=foo; echo after\necho next",
			"next", "after",
		},
		{
			// The control that says the subscript is the whole of it: the
			// plain write still aims the reference.
			"a plain write still aims it",
			"typeset -n ref\nref=foo\ntypeset -p ref",
			`declare -n ref="foo"`, "not a valid identifier",
		},
		{
			// And an aimed reference takes the element through, which is the
			// row this must not have moved.
			"an aimed reference takes the element through",
			"declare -a v=(p q)\ntypeset -n ref=v\nref[0]=foo\ntypeset -p v",
			`declare -a v=([0]="foo" [1]="q")`, "not a valid identifier",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
