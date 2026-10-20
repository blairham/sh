// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Two refusals this shell makes inside `[[ ]]`, both of which this tree had
// at the wrong status and one at the wrong wording.
//
// Measured 2026-09-18 on zsh 5.9.2, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` (#3279, #3280).

// An empty `=~` right operand: this shell says what its engine said and
// leaves the condition **false**, where bash calls the same operand a failure
// of the construct and ends at 2.
func TestAnEmptyRegexOperandIsARefusedMatchRatherThanAFailure(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, "[[ abc =~ \"\" ]]\nprintf 'st=%s' \"$?\"\n")
	if want := "failed to compile regex: empty (sub)expression"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to hold %q", out, want)
	}
	if want := "st=1"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to hold %q — the condition is false, not failed", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0 — the printf after it is what ran last", st)
	}
	// The controls that say the operator works, so the rows above are about
	// the refusal and not about `=~`.
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"[[ abc =~ b ]]", "st=0"},
		{"[[ abc =~ x ]]", "st=1"},
	} {
		out, _ := runZsh(t, dir, tc.src+"\nprintf 'st=%s' \"$?\"\n")
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// A process substitution reaches every operand of every operator here and is
// refused at each, the input being abandoned — and the status it leaves is a
// property of the expression rather than of the word the sentence names.
func TestAProcessSubstitutionIsRefusedAtEveryConditionOperand(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src    string
		named  string
		status int
		why    string
	}{
		{`[[ -e <(echo x) ]]`, "<(echo x)", 1, "a file test's operand"},
		{`[[ -n <(echo x) ]]`, "<(echo x)", 1, "a string test's"},
		{`[[ <(echo x) == x ]]`, "<(echo x)", 1, "the left side of a comparison"},
		{`[[ x -nt <(echo x) ]]`, "<(echo x)", 1, "the right side of one that is not a pattern"},
		{`[[ ! -e <(echo x) ]]`, "<(echo x)", 1, "under a negation"},
		{`[[ ( -e <(echo x) ) ]]`, "<(echo x)", 1, "inside a group"},
		{`[[ x == <(echo x) ]]`, "<(echo x)", 2, "the right side of a pattern comparison"},
		{`[[ x != <(echo x) ]]`, "<(echo x)", 2, "which the negated spelling is too"},
		{`[[ x == >(echo x) ]]`, ">(echo x)", 1, "where the output spelling is not"},
		{`[[ x == =(echo x) ]]`, "=(echo x)", 1, "nor the file one"},
		{`[[ <(echo x) == <(echo y) ]]`, "<(echo x)", 2, "an input substitution behind the operator"},
		{`[[ <(echo x) == >(echo y) ]]`, "<(echo x)", 1, "and one of the other spelling there"},
	} {
		src := "printf 'start\\n'\n" + tc.src + "\nprintf 'after\\n'\n"
		out, st := runZsh(t, dir, src)
		want := "start\nprocess substitution " + tc.named + " cannot be used here"
		if !strings.HasPrefix(out, "start\n") || !strings.Contains(out, "process substitution "+tc.named+" cannot be used here") {
			t.Errorf("%s: output = %q, want it to hold %q — %s", tc.src, out, want, tc.why)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s: the input was not abandoned — %q", tc.src, out)
		}
		if st != tc.status {
			t.Errorf("%s: status %d, want %d — %s", tc.src, st, tc.status, tc.why)
		}
	}
	// And the refusal is lazy, because the condition is: an operand a
	// short-circuit never reaches is never refused.
	out, st := runZsh(t, dir, "[[ x == y && -e <(echo x) ]]\nprintf 'st=%s' \"$?\"\n")
	if out != "st=1" || st != 0 {
		t.Errorf("a short-circuit: got %q (status %d), want \"st=1\" at 0", out, st)
	}
}
