// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestReadJudgesEveryNameFirst: the whole list is judged before anything is
// read, so a bad name anywhere fills nothing and leaves the line for the next
// reader. Measured 2026-10-03 in the pinned image.
func TestReadJudgesEveryNameFirst(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`printf 'X Y Z\nNEXT\n' | { c=keep; read a 1bad c; echo "st=$? a=[$a] c=[$c]"; read n; echo "n=[$n]"; }`,
			"st=1 a=[] c=[keep]\nn=[X Y Z]\n",
		},
		{`printf 'AAA\nBBB\n' | { (read 1bad); read next; echo "next=[$next]"; }`, "next=[AAA]\n"},
		{`printf 'XYZW\n' | { b=keep; read -n 3 a 1bad b; echo "st=$? a=[$a] b=[$b]"; }`, "st=1 a=[] b=[keep]\n"},
	} {
		out, _ := run(t, tc.src)
		if !strings.HasSuffix(out, tc.want) || !strings.Contains(out, "'1bad': bad variable name") {
			t.Errorf("%s\n got %q\nwant the complaint and %q", tc.src, out, tc.want)
		}
	}
}
