// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An operand of `[[ ]]` whose expansion failed ends the condition, and what
// it ends beyond that is the dialect's existing answer for a failed expansion
// — which is why the panel needs no axis of its own here.
//
// The rows are asked through Semantics.FailedExpansionAbandonsTheLine, the
// axis that already carries it, and never by a shell's name. Measured over
// `[[ $((1/0)) -eq 0 ]]` with a `printf` on either side: the column that
// abandons the line reports the division and carries on at the next line,
// where the columns that end the shell stop there.
//
// So the status is read on the **next** line in every row below. The line the
// condition stands on is given up either way, which is what the axis means,
// and a `printf` written behind it on the same line would be measuring that
// rather than the condition.

func condArithFails(abandons Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.FailedExpansionAbandonsTheLine = abandons
		// The status a failed expansion leaves, which is the axis that
		// already answers it and is asserted elsewhere: named here so the
		// rows below are about the condition rather than about that number.
		s.FatalErrorStatusIsOne = Yes
		r.Semantics = &s
	}
}

func TestAFailedOperandExpansionLeavesTheConditionFalse(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		why  string
	}{
		// The reproduction: an empty operand would compare equal to zero,
		// and the failure is what stops it doing so.
		{`[[ $((1/0)) -eq 0 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "the comparison does not hold"},
		// The condition is abandoned whole rather than the primary being
		// false, which these two say and a plain false could not: a negated
		// false is true, and a `||` whose right side holds is true.
		{`[[ ! $((1/0)) -eq 0 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "a negation does not flip it"},
		{`[[ $((1/0)) -eq 0 || 1 -eq 1 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "nor does a `||` that would hold"},
		// Either operand, and the other operator forms.
		{`[[ 0 -eq $((1/0)) ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "the right operand"},
		{`[[ $((1/0)) -lt 5 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "a different comparison"},
		{`[[ -n $((1/0)) ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "a unary operand"},
		// The two rows a plain empty operand would answer the other way,
		// which is what separates "the expansion failed" from "the text is
		// empty": `-z` holds over an empty string and `== ""` matches one.
		{`[[ -z $((1/0)) ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "an emptiness test the empty text would pass"},
		{`[[ $((1/0)) == "" ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "a pattern the empty text would match"},
		{`[[ $((1/0)) != x ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "and a negated one it would too"},
		{`[[ $((1/0)) ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=1]", "and a bare word"},
		// The `||` that never reaches it is the control: no complaint at
		// all, and the condition holds.
		{`[[ 1 -eq 1 || $((1/0)) -eq 0 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=0]", "a short-circuit reaches nothing"},
		// And an **empty** operand is not this. The row is about the
		// expansion having failed and not about the text it left behind,
		// which is exactly what this shell used to answer.
		{`[[ "" -eq 0 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=0]", "an empty operand is still zero"},
		{`[[ $nosuch -eq 0 ]]` + "\n" + `printf "[st=%d]" "$?"`, "[st=0]", "and so is an unset one"},
	} {
		out, _ := run(t, tc.src, condArithFails(Yes))
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("%s = %q, want it to end %q — %s", tc.src, out, tc.want, tc.why)
		}
	}
}

// The left operand is settled before the right one is expanded, so a failure
// in the first stops the second from running at all — which is observable,
// because an operand may hold a command.
func TestAFailedLeftOperandStopsTheRightOneRunning(t *testing.T) {
	out, _ := run(t, `[[ $((1/0)) -eq $(printf "[RIGHT]"; echo 1) ]]`+"\n"+`printf "[st=%d]" "$?"`,
		condArithFails(Yes))
	if strings.Contains(out, "[RIGHT]") {
		t.Errorf("got %q, want the right operand's command not to have run", out)
	}
	if !strings.HasSuffix(out, "[st=1]") {
		t.Errorf("got %q, want it to end [st=1]", out)
	}
}

// And what the failure ends past the condition is the axis's, not this
// construct's: the column that ends the shell ends it here too.
func TestAFailedOperandEndsWhatTheDialectEnds(t *testing.T) {
	src := `printf "[a]"; [[ $((1/0)) -eq 0 ]]; printf "[b]"` + "\n" + `printf "[c]"`
	out, _ := run(t, src, condArithFails(Yes))
	if !strings.Contains(out, "[a]") || !strings.HasSuffix(out, "[c]") {
		t.Errorf("abandoning the line: got %q, want [a] and then [c]", out)
	}
	if strings.Contains(out, "[b]") {
		t.Errorf("abandoning the line: got %q, want nothing past the condition on its line", out)
	}
	out, _ = run(t, src, condArithFails(No))
	if strings.Contains(out, "[c]") {
		t.Errorf("ending the shell: got %q, want nothing past the line", out)
	}
	if !strings.Contains(out, "[a]") {
		t.Errorf("ending the shell: got %q, want the command in front of it to have run", out)
	}
}
