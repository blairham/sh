// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestATypedValueFailureFromABuiltinCostsWhatAnAssignmentsDoes pins bash's
// `getopts` and `read` over a typed name and a value that will not evaluate:
// the complaint, and nothing after it on the line, at the status an
// assignment's failure leaves. Measured 2026-10-15 on bash 5.3.20 under `-c`.
// See interp.Semantics.GetoptsTypedValueFailureIsAFreeze and
// ReadTypedValueFailureIsAnAssignmentFailure. A `-c` string, because from a
// script file that cost is the rest of the line: see
// TestATypedValueFailureGivesUpTheLine.
func TestATypedValueFailureFromABuiltinCostsWhatAnAssignmentsDoes(t *testing.T) {
	for _, tc := range []struct{ src, sentence string }{
		{`typeset -i n=5; getopts x n -y 2>/dev/null; echo "st=$? [$n]"`, ""},
		{`typeset -i n=5; getopts x n; echo "st=$? [$n]"`, "getopts: ?: arithmetic syntax error"},
		{"typeset -i n=5; read n m <<E\n1+ b\nE\necho \"[$?] [$n] [$m]\"", "read: 1+: arithmetic syntax error"},
	} {
		out, st := posixRouteStatus(t, tc.src, interp.RouteCommandString)
		if strings.Contains(out, "st=") || strings.Contains(out, "[") || st != 1 {
			t.Errorf("%s\n got %q (%d), want the complaint alone at 1", tc.src, out, st)
		}
		if !strings.Contains(out, tc.sentence) {
			t.Errorf("%s\n got %q, want %q in it", tc.src, out, tc.sentence)
		}
	}
}
