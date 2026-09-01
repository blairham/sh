// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// An expansion inside an expression is *textual* and happens first, so the
// result is the expression. That is why an expression containing one has no
// tree until it runs — `$x$y` with x=`1+` and y=`2` is `1+2`, and no tree
// built from `$x$y` could be.
func TestAnExpansionInAnExpressionHappensFirst(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x='1+'; y=2; echo $(( $x$y ))`, "3"},
		// Two halves of an operator, which only works if the substitution is
		// text rather than a value slotted into a tree.
		{`op='*'; echo $(( 3 $op 4 ))`, "12"},
		// The parameters a script actually does arithmetic on.
		{`set -- 5 7; echo $(( $2-2 ))`, "5"},
		{`set -- a b c; echo $(( $# + 1 ))`, "4"},
		{`false; echo $(( $? + 1 ))`, "2"},
		{`x=4; echo $(( ${x} + 1 ))`, "5"},
		{`x=5; echo $(( $(echo 2) + x ))`, "7"},
		// Nothing to substitute, so the name is the evaluator's business.
		{`x=7; echo $(( x + 1 ))`, "8"},
		{`echo $(( 1 + 2 ))`, "3"},
		// An empty expression is zero rather than a failure.
		{`echo $(( ))`, "0"},
		// The same in the command form and in a loop header.
		{`x=2; (( $x > 1 )) && echo yes`, "yes"},
		{`n=3; for ((i=0;i<$n;i++)); do printf %s $i; done; echo`, "012"},
	} {
		if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// The condition and the step are read again each time round, because what they
// expand to can change between iterations.
func TestALoopHeaderIsExpandedEachTime(t *testing.T) {
	out, _ := run(t, `limit=1; for ((i=0;i<$limit;i++)); do printf %s $i; limit=3; done; echo`, nil)
	if got, want := strings.TrimSpace(out), "012"; got != want {
		t.Errorf("got %q, want %q — the condition is re-read", got, want)
	}
}

// An expression that will not read is reported when it runs, which is the only
// time it can be: until it is expanded there is nothing to read.
func TestAnUnreadableExpandedExpressionFailsWhenItRuns(t *testing.T) {
	out, _ := run(t, `op='+'; echo before; echo $(( 1 $op ))`, nil)
	if !strings.Contains(out, "before") {
		t.Errorf("got %q, want the earlier command to have run", out)
	}
	if !strings.Contains(out, "error") && !strings.Contains(out, "expected") {
		t.Errorf("got %q, want the failure reported", out)
	}
}
