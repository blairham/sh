// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration whose operand carries a **subscript** leaves a name reference
// standing here, and the element goes through it.
//
// This is the other answer to Semantics.NamerefDroppedByASubscriptedOperand —
// bash takes the attribute off and lands the element on the name — and the
// aimed row is where the two really part rather than only in the words.
//
// Measured 2026-09-23 against ksh93u+ 2012-08-01, `env -i` from a script file:
//
//	array=(p q); typeset -n xref=array; typeset -a xref[1]=one
//	  xref is still the reference, and array is (p one)
//	typeset -n xref; typeset -a xref[1]=one
//	  xref: no reference name, and the script ends at 1
//
// The second was `typeset -n xref=one` here — the reference aimed at the
// value, which is nobody's answer: a subscript never aims one (#4178).
func TestASubscriptedDeclarationOperandLeavesAReferenceStanding(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row that makes this a dialect's answer rather than the
			// shell's: the element goes through the reference.
			"an aimed reference keeps the attribute and takes the element",
			"array=(p q)\ntypeset -n xref=array\ntypeset -a xref[1]=one\ntypeset -p xref array",
			"typeset -n xref=array\ntypeset -a array=(p one)", "",
		},
		{
			// A reference with nothing to point at has no name for the
			// element to land on.
			"an unaimed reference has no name for the element",
			"typeset -n xref\ntypeset -a xref[1]=one\ntypeset -p xref",
			"xref: no reference name", "",
		},
		{
			// And that refusal **ends the script**, which is what a write
			// through an unaimed reference costs here — so the line after it
			// is the assertion rather than a second row.
			"and the refusal ends the script",
			"typeset -n xref\ntypeset -a xref[1]=one\necho REACHED",
			"xref: no reference name", "REACHED",
		},
		{
			// The literal spelling is the other road, and there the
			// attribute goes in both shells — silently in this one.
			"the literal spelling drops the attribute silently",
			"typeset -n xref\ntypeset -a xref=([1]=one)\ntypeset -p xref",
			"typeset -a xref=([1]=one)", "",
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
