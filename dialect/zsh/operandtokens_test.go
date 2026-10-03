// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFlaggedOperandKeepsItsTokens pins that the word a `-` or `+`
// substituted under a flag group comes to its words with their pattern
// characters still live: the braces make words first, the group runs on each,
// the single `q` leaves the pattern characters as they are and every other
// quoting style quotes them, and the matching and an `=` head come last
// (#5151, a chunk of D04parameter.ztst). Measured 2026-10-03 on zsh 5.9.2
// under `-f`, in a directory holding `xay` and `xby`.
func TestAFlaggedOperandKeepsItsTokens(t *testing.T) {
	const setup = "print -n >xay; print -n >xby\n"
	for _, tc := range []struct{ src, want string }{
		{`print -l ${(U):-{b,c}} ${(qq):-{b,c}} ${(j:-:):-{b,c}}`, "B\nC\n'b'\n'c'\nb-c\n"},
		{`print -r -- ${(qq):-x*} ${(q+):-x?y} ${(q):-xa*} ${(q):-x?y}`, "'x*' 'x?y' xay xay xby\n"},
		{`print -r -- ${(U):-x*}`, "zsh:2: no matches found: X*\n"},
		{`print -r -- ${(q):-=} ${(q):-#a} ${(q):-a^b} ${(q):-'xa*'} ${(qq):-=}a`, "= #a a^b xa\\* '='a\n"},
		{`print -r - ${(qq):-a= {b,c}*}, ${(q-):-a= {b,c}*}`, "'a= b*' 'a= c*', 'a= b*' 'a= c*'\n"},
		{`print -r - ${(q):-a= {b,c}*}`, "zsh:2: no matches found: a=\\ b*\n"},
		{`print -r - ${(q):-=}a`, "zsh:2: a not found\n"},
		{`PATH=/bin:/usr/bin; print -r - ${(q):-=}ls x${(q):-=}ls`, "/bin/ls x=ls\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
