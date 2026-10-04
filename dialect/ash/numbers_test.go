// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestNumbersAshReadsItsOwnWay: three readings of a number BusyBox ash 1.37.0
// does not share with dash, measured 2026-10-03 in the pinned image. A signed
// `shift` count is no number and ends the script at 2; `return` masks what it
// read to eight bits; and `let` fails at 2, naming itself in front of a math
// complaint.
func TestNumbersAshReadsItsOwnWay(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`set -- a b c; shift +1; echo "st=$? n=$#"`, "Illegal number: +1", 2},
		{`set -- a b c; shift -0; echo "st=$? n=$#"`, "Illegal number: -0", 2},
		{`set -- a b c; shift 01; echo "st=$? n=$#"`, "st=0 n=2", 0},
		{`f(){ return 300; }; f; echo "st=$?"`, "st=44", 0},
		{`let; echo "st=$?"`, "expression expected\nst=2", 0},
		{`let 1/0; echo "st=$?"`, ": let: line 1: divide by zero\nst=2", 0},
		{`let 0; echo "st=$?"`, "st=1", 0},
	} {
		out, st := run(t, tc.src)
		if !strings.Contains(out, tc.want) || st != tc.status {
			t.Errorf("%s\n got %q (status %d)\nwant %q in it (status %d)", tc.src, out, st, tc.want, tc.status)
		}
	}
}
