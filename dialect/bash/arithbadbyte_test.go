// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAnUnreadableByteWhereAnOperatorBelonged — bash words a byte that begins
// no token as a missing operand behind a group and as a bad operator behind
// anything else, and reads the token behind a second operand before refusing
// the operand. Measured 2026-10-04 on bash 5.3.20; see arithmetic.md.
func TestAnUnreadableByteWhereAnOperatorBelonged(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{"(1)@", `operand expected (error token is "@ ")`},
		{"(1) [2]", `operand expected (error token is "[2] ")`},
		{"-(1)@+2", `operand expected (error token is "@+2 ")`},
		{"x@", `invalid arithmetic operator (error token is "@ ")`},
		{"1[2]", `invalid arithmetic operator (error token is "[2] ")`},
		{"1 x@", `invalid arithmetic operator (error token is "@ ")`},
		{"(1)x[2]@", `invalid arithmetic operator (error token is "@ ")`},
		{`(r)bet\a`, `invalid arithmetic operator (error token is "\a ")`},
		// The controls: nothing unreadable behind the operand.
		{"1 x y", `syntax error in expression (error token is "x y ")`},
		{"(1)x", `syntax error in expression (error token is "x ")`},
	} {
		_, errs, _ := runAlias(t, "x=1; echo $(( "+c.expr+" ))")
		if !strings.Contains(errs, c.want) {
			t.Errorf("$(( %s )): said %q, want %q", c.expr, errs, c.want)
		}
	}
}

// TestAnEmptyArithSubscriptIsReportedTwice — every read of `a[]` in an
// expression is two identical lines in bash 5.3.20, measured 2026-10-04.
func TestAnEmptyArithSubscriptIsReportedTwice(t *testing.T) {
	_, errs, _ := runAlias(t, "a=(5); x=$(( a[] * a[] )); echo $x")
	if got := strings.Count(errs, "a[]: bad array subscript\n"); got != 4 {
		t.Errorf("said %q, want the complaint four times", errs)
	}
}

// TestAPositionalRangeIsNamedAsWritten — `${@:#}` is refused naming `@`, not
// the `@[@]` the list reading carries inside. Measured 2026-10-04 on 5.3.20.
func TestAPositionalRangeIsNamedAsWritten(t *testing.T) {
	for _, src := range []string{`set -- a; echo ${@:#}`, `set -- a; echo "${*:1:#}"`} {
		_, errs, _ := runAlias(t, src)
		name := src[strings.Index(src, "{")+1 : strings.Index(src, "{")+2]
		if !strings.Contains(errs, "line 1: "+name+": #: arithmetic syntax error") {
			t.Errorf("%s: said %q, want it named %q", src, errs, name)
		}
	}
}
