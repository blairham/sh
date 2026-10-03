// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestATildeNamingNoUserIsRefused pins that a `~name` neither the named
// directories nor the user database answers is refused as an unmatched pattern
// is, and stops the script, while one that is no name at all, or quoted, or
// under `nonomatch`, is left as written (#5646). Through the binary, because
// the user database is the front end's hook. Measured 2026-10-03 on zsh 5.9.2
// under -f.
func TestATildeNamingNoUserIsRefused(t *testing.T) {
	const refused = "zsh:1: no such user or named directory: shnosuch5646\n"
	for _, tc := range []struct {
		src, out, errs string
		status         int
	}{
		{`print ~shnosuch5646; print after`, "", refused, 1},
		{`print ~shnosuch5646/x; print after`, "", refused, 1},
		{`x=~shnosuch5646; print after`, "", refused, 1},
		{`x=a:~shnosuch5646; print after`, "", refused, 1},
		{`a=(~shnosuch5646); print after`, "", refused, 1},
		{`[[ ~shnosuch5646 = x ]]; print after`, "", refused, 1},
		{
			`f() { print ~no.such5646; }; f; print after`,
			"", "f: no such user or named directory: no.such5646\n", 1,
		},
		// A subshell's refusal ends the subshell, at 1.
		{`print $(print ~shnosuch5646; print in) after $?`, "after 1\n", refused, 0},
		// No name at all, quoted, or not at the head: the word as written.
		{
			`print ~no@such5646 "~shnosuch5646" x~shnosuch5646 a=~shnosuch5646; print after`,
			"~no@such5646 ~shnosuch5646 x~shnosuch5646 a=~shnosuch5646\nafter\n", "", 0,
		},
		{`print ~shnosuch5646:x; print after`, "~shnosuch5646:x\nafter\n", "", 0},
		{
			`setopt nonomatch; print ~shnosuch5646 a:~shnosuch5646; print after`,
			"~shnosuch5646 a:~shnosuch5646\nafter\n", "", 0,
		},
		// An assignment's colon reaches the user database as the head does.
		{
			`y=~root; x=a:~root:~root/b; [[ $x == a:$y:$y/b && $y != '~root' ]] && print ok`,
			"ok\n", "", 0,
		},
	} {
		out, errs, status := runZsh(t, "-fc", tc.src)
		if out != tc.out || errs != tc.errs || status != tc.status {
			t.Errorf("%s\n got %q %q %d\nwant %q %q %d", tc.src, out, errs, status, tc.out, tc.errs, tc.status)
		}
	}
}
