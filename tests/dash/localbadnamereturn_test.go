// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A bad `local` name carrying a value is taken in silence and refused —
// fatally, naming the last such operand and no builtin — when the function
// returns. Without a value it is refused at once. Measured 2026-10-03 on dash
// 0.5.12; see Semantics.LocalBadNameWithAValueFailsAtReturn.
func TestABadLocalNameWithAValueFailsAtTheReturn(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{"f() { local 1x=5; echo in=$?; }; f; echo st=$?", "in=0\n1x: bad variable name\n", 2},
		{"f() { local 1x=5 2y=3; echo in; }; f; echo st=$?", "in\n2y: bad variable name\n", 2},
		{"f() { local 1x=5; exit 4; }; f; echo st=$?", "", 4},
		{"f() { local 1x; echo in; }; f; echo st=$?", "local: 1x: bad variable name\n", 2},
		// A subscripted name is one of them: measured 2026-10-04.
		{"f() { local a[1]=v; echo in=$?; }; f; echo st=$?", "in=0\na[1]: bad variable name\n", 2},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if st != tc.status || !strings.HasSuffix(strings.ReplaceAll(out, "sh: 1: ", ""), tc.want) {
				t.Errorf("= %q status %d, want it to end %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}
