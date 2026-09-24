// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A declaration whose operand carries a **subscript** takes a name reference's
// attribute off, and writes the element on that name rather than through the
// reference.
//
// `typeset -n xref; typeset -a xref[1]=one` warns and leaves `xref` an
// ordinary array here. This shell aimed the reference at `one` instead — the
// subscript dropped, the letter dropped, and nothing written anywhere.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	                                   bash                        before
//	typeset -n xref
//	  typeset -a xref[1]=one           warning, declare -a          declare -n
//	                                   xref=([1]="one") at 0        xref="one"
//	array=(p q); typeset -n xref=array
//	  typeset -a xref[1]=one           the same, and `array`        the element
//	                                   still holds (p q)            went through
//
// The aimed row is the one that makes this a dialect's answer rather than the
// shell's: ksh93u+ writes through the reference there and keeps it. See
// Semantics.NamerefDroppedByASubscriptedOperand (#4178).
func TestASubscriptedDeclarationOperandTakesAReferenceOff(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row `nameref15.sub` grades, twice: a reference with
			// nothing to point at.
			"an unaimed reference gives the attribute up",
			"typeset -n xref\ntypeset -a xref[1]=one\ndeclare -p xref",
			`declare -a xref=([1]="one")`,
		},
		{
			"and says so",
			"typeset -n xref\ntypeset -a xref[1]=one",
			"warning: xref: removing nameref attribute",
		},
		{
			// The other half of that row, and the half that makes this a
			// dialect's answer: an aimed reference goes the same way here
			// and the target is left alone.
			"an aimed reference gives it up too, and the target is untouched",
			"array=(p q)\ntypeset -n xref=array\ntypeset -a xref[1]=one\ndeclare -p xref array",
			"declare -a xref=([1]=\"one\")\ndeclare -a array=([0]=\"p\" [1]=\"q\")",
		},
		{
			// No letter needed: the subscript is what does it.
			"a plain declaration with a subscript does it too",
			"typeset -n xref\ntypeset xref[1]=one\ndeclare -p xref",
			`declare -a xref=([1]="one")`,
		},
		{
			// And the table letter takes the same road.
			"the table letter takes the same road",
			"typeset -n xref\ntypeset -A xref[k]=one\ndeclare -p xref",
			`declare -A xref=([k]="one" )`,
		},
		{
			// The line runs on at 0 — this is a warning and not a refusal.
			"the line runs on at 0",
			"typeset -n xref\ntypeset -a xref[1]=one\nprintf '<%d>' $?\ndeclare -p xref",
			`<0>declare -a xref=([1]="one")`,
		},
		{
			// The control that says this is the *operand's* subscript and
			// not the name being a reference: the plain statement keeps the
			// reference and writes through it.
			"the plain statement is the other road and keeps the reference",
			"array=(p q)\ntypeset -n xref=array\nxref[1]=one\ndeclare -p xref array",
			"declare -n xref=\"array\"\ndeclare -a array=([0]=\"p\" [1]=\"one\")",
		},
		{
			// And the control that says a name which is not a reference is
			// untouched by any of this.
			"a name that is not a reference is untouched",
			"typeset -a xref[1]=one\ndeclare -p xref",
			`declare -a xref=([1]="one")`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
