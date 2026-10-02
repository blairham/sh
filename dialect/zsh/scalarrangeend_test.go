// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A search as the second end of a range over a string names where its match
// ends rather than where it begins. See interp.Runner.scalarSearchEnd.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc` (#5153).
func TestASearchEndingARangeOverAString(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the end is where the match ends", "a=abcdefg; print ${a[(r)cd,(r)ef]}", "cdef\n"},
		{"a pattern's end", "a=abcdefg; print ${a[1,(r)c?e]}", "abcde\n"},
		{"forward, the shortest", "a=abcdefg; print ${a[1,(r)c*]} ${a[1,(r)c(d|de)]}", "abc abcd\n"},
		{"an empty match ends before the first", "a=abcdefg; print ${a[2,(r)*]}x", "x\n"},
		{"backward, the longest", "a=abcdefg; print ${a[1,(R)c*]} ${a[2,(R)??]}", "abcdefg bcdefg\n"},
		{"the nth match from the end", "a=abcdefg; print ${a[1,(Rn:2:)?]}", "abcdef\n"},
		{"a start names the first end", "a=abcdefg; print ${a[1,(rb:4:)?]} ${a[1,(Rb:4:)??]} ${a[1,(Rb:4:)d]}x", "abc abc x\n"},
		{"a start from the end", "a=abcdefg; print ${a[1,(rb:-2:)?]}", "abcde\n"},
		{"exact", "a=abcdefg; print ${a[1,(re)cd]}", "abcd\n"},
		{"an index letter names the end too", "a=abcdefg; print ${a[(r)cd,(i)ef]}", "cdef\n"},
		{"the first end keeps the match's start", "a=abcdefg; print ${a[(r)cd,-1]}", "cdefg\n"},
		{"misses", "a=abcdefg; print ${a[1,(r)zz]}x ${a[1,(R)zz]}x", "abcdefgx x\n"},
		{"by characters", "a=ténébreux; print ${a[(r)én,(r)éb]} ${a[(r)é,(R)é]}", "énéb éné\n"},
		{"an index letter the same", "a=abcdefg; print ${a[2,(i)ef]} ${a[2,(I)ef]} ${a[2,(i)e?]}", "bcdef bcdef bcdef\n"},
		{"an index letter's misses", "a=abcdefg; print ${a[2,(i)zz]}x ${a[2,(I)zz]}x", "bcdefgx x\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("%s\n got %q, %q\nwant %q", c.src, out, errs, c.want)
			}
		})
	}
}
