// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `let` is `(( ))` with the expression as a word.
func TestLetEvaluates(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`let "x = 2 + 3"; echo $x`, "5"},
		{`let x=1+1; echo $x`, "2"},
		{`x=3; let "x*=2"; echo $x`, "6"},
		{`let "x = 2 * (3 + 4)"; echo $x`, "14"},
		// Every argument is evaluated, for its side effect as much as its
		// value.
		{`let x++ y=2; echo "$x $y"`, "1 2"},
		{`let a=1 b=2 c=3; echo "$a$b$c"`, "123"},
	} {
		if out, _ := run(t, c.src, nil); strings.TrimSpace(out) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
}

// The status is the surprising part: an expression coming out zero is a
// *failure*, because a shell reports false for it.
func TestLetReportsZeroAsFalse(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{
		{`let "x=5"`, 0},
		{`let "x=0"`, 1},
		{`let "x=-1"`, 0},
		{`let "1 == 1"`, 0},
		{`let "1 == 2"`, 1},
		// Only the last one decides, so a leading zero does not fail.
		{`let a=0 b=1`, 0},
		{`let a=1 b=0`, 1},
	} {
		if _, st := run(t, c.src, nil); st != c.want {
			t.Errorf("%s = %d, want %d", c.src, st, c.want)
		}
	}
	// And the assignment still happened even where the status says failure.
	out, _ := run(t, `let "x=0"; echo "[$x]"`, nil)
	if strings.TrimSpace(out) != "[0]" {
		t.Errorf("got %q, want the assignment to have happened", out)
	}
}

// With nothing to evaluate it complains rather than succeeding quietly.
func TestLetWithNoExpression(t *testing.T) {
	out, st := run(t, `let`, nil)
	if st == 0 {
		t.Errorf("status 0 (said %q), want a failure", out)
	}
	if out == "" {
		t.Errorf("said nothing")
	}
}
