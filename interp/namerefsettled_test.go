// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a reference is aimed **at**, and who a refusal through one is about.
// Two questions one declaration answers, and neither is about where the
// *name* is resolved — that is a redirect in every column (#3124, #3172,
// #3173).

// runSettled runs src where a reference's target is settled at the
// declaration: a subscript evaluated then, a chain followed then.
func runSettled(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	sem.NamerefTargetResolvedWhenAimed = Yes
	sem.ArrayBaseIsZero = Yes
	// The give-up a refused subscript costs, which the last case here asks
	// about and which is its own axis: the whole input, as the column that
	// settles the target answers it.
	sem.BadSubscriptToADeclaration = BadSubscriptEndsTheScript
	return runNamerefWith(t, sem, src)
}

// A subscript is arithmetic, so settling the target settles the number — and
// what the subscript *named* stops mattering afterwards.
func TestASettledReferenceRecordsTheNumberItsSubscriptCameTo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a subscript naming a variable",
			src:  "a=(x y z)\ni=2\ntypeset -n r=a[i]\ntypeset -p r\ni=0\necho \"[$r]\"",
			want: "declare -n r='a[2]'\n[z]\n",
		},
		{
			name: "an expression",
			src:  "a=(x y z)\ntypeset -n r=a[1+1]\ntypeset -p r",
			want: "declare -n r='a[2]'\n",
		},
		{
			// Counted forwards from the end the array has now, which is the
			// position the element store would have used.
			name: "a negative subscript",
			src:  "a=(x y z)\ntypeset -n r=a[-1]\ntypeset -p r",
			want: "declare -n r='a[2]'\n",
		},
		{
			// A **key** is not an expression and is left as written, in this
			// column as much as in the other.
			name: "a table's key",
			src:  "typeset -A m=([k]=1)\ntypeset -n r=m[k]\ntypeset -p r\necho \"[$r]\"",
			want: "declare -n r='m[k]'\n[1]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSettled(t, tc.src)
			if out != tc.want || status != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, status, tc.want)
			}
		})
	}
	// And the other answer keeps the text, which is the control: the same
	// source read again after the subscript's variable moved is a different
	// element.
	out, _ := runNameref(t, "a=(x y z)\ni=2\ntypeset -n r=a[i]\ntypeset -p r\ni=0\necho \"[$r]\"")
	if out != "declare -n r='a[i]'\n[x]\n" {
		t.Errorf("out %q, want the subscript looked up again at the read", out)
	}
}

// A subscript that will not evaluate is **this declaration's** failure where
// the target is settled here, and the complaint is the builtin's own.
func TestASettledReferenceRefusesASubscriptItCannotEvaluate(t *testing.T) {
	for _, sub := range []string{"a[@]", "a[*]", "a[1+]"} {
		t.Run(sub, func(t *testing.T) {
			out, _ := runSettled(t, "a=(x y)\ntypeset -n r="+sub+"\necho AFTER")
			if !strings.Contains(out, strings.TrimSuffix(sub[2:], "]")+":") {
				t.Errorf("out %q does not report the subscript it could not evaluate", out)
			}
			if strings.Contains(out, "AFTER") {
				t.Errorf("out %q carried on past a declaration it could not make", out)
			}
		})
	}
	// The whole-array target stands under the other answer, which is what
	// says the refusal is the evaluation's and not a rule about brackets.
	out, _ := runNameref(t, "a=(x y)\ntypeset -n r=a[@]\necho \"[$r]\"")
	if !strings.Contains(out, "[x y]") {
		t.Errorf("out %q, want the whole array read through the reference", out)
	}
}

// A chain is followed to its end at the declaration, so re-aiming the middle
// link afterwards leaves this one where it was.
func TestASettledReferenceFollowsTheChainOnce(t *testing.T) {
	src := "u=1\nw=9\ntypeset -n s=u\ntypeset -n s2=s\ntypeset -p s2\ntypeset -n s=w\necho \"[$s2]\"\ntypeset -p s2"
	out, _ := runSettled(t, src)
	if out != "declare -n s2='u'\n[1]\ndeclare -n s2='u'\n" {
		t.Errorf("out %q, want the chain flattened at the declaration", out)
	}
	// The other answer records the name that was written and walks it at
	// every use, so the same source follows the re-aim.
	out, _ = runNameref(t, src)
	if out != "declare -n s2='s'\n[9]\ndeclare -n s2='s'\n" {
		t.Errorf("out %q, want the middle name kept and walked", out)
	}
}

// A refusal a declaration reaches **through** a reference may be spoken of
// under the name the script wrote. Only where the declaration carries a
// value, which is the pair that makes it a rule.
func TestADeclarationThroughAReferenceCanNameItsOwnOperand(t *testing.T) {
	sem := namerefAimSemantics()
	sem.NamerefTargetResolvedWhenAimed = No
	sem.DeclarationThroughAReferenceNamesTheOperand = Yes
	sem.ReadonlyReassignmentFatal = No
	sem.ReadonlyReassignmentByDeclarationFatal = No
	sem.NumericTypeLetterRetypesAFrozenName = No
	sem.AttributeOverAFrozenNameIsRefused = Yes
	sem.DeclarationTakesAnAppendOperand = Yes
	for _, tc := range []struct{ name, src, want string }{
		{"a valueless letter", "typeset -i s", "u: readonly variable"},
		{"a value", "typeset s=9", "s: readonly variable"},
		{"a letter and a value", "typeset -i s=9", "s: readonly variable"},
		{"an append", "typeset s+=9", "s: readonly variable"},
		{"a plain assignment", "s=9", "u: readonly variable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runNamerefWith(t, sem, "u=1\nreadonly u\ntypeset -n s=u\n"+tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("out %q does not say %q", out, tc.want)
			}
		})
	}
	// The redirect itself is not in question: a frozen **reference** is
	// walked straight past, so the row above is about the name in the
	// sentence and not about where the check is made.
	out, status := runNamerefWith(t, sem, "u=1\ntypeset -rn s=u\ntypeset s=9\ntypeset -p u")
	if status != 0 || !strings.Contains(out, `u='9'`) {
		t.Errorf("out %q status %d, want the write through a frozen reference taken", out, status)
	}
	// And the other answer names the cell the write would have landed in,
	// value or no value.
	sem.DeclarationThroughAReferenceNamesTheOperand = No
	out, _ = runNamerefWith(t, sem, "u=1\nreadonly u\ntypeset -n s=u\ntypeset s=9")
	if !strings.Contains(out, "u: readonly variable") {
		t.Errorf("out %q does not name the target", out)
	}
}
