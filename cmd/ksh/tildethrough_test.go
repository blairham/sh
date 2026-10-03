// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestATildeNameRunsThroughQuotesAndEndsAtAWrittenSlash pins ksh93's reading
// of a tilde prefix that holds a quote or an expansion: the name is read
// through them, and ends only at a `/` the word writes — one an expansion
// produces or a backslash quotes is part of the name, which then names
// nothing (#5689). Through the binary, because `~root` is the user
// database's. Measured 2026-10-03 on ksh93u+ 2012-08-01.
func TestATildeNameRunsThroughQuotesAndEndsAtAWrittenSlash(t *testing.T) {
	const pre = "r=~root x=root y=root/x m=- e= q=/yy; cd /tmp; OLDPWD=/old\n"
	for _, tc := range []struct{ src, out string }{
		{`[[ ~"root" == "$r" && "$r" != '~root' && ~\root == "$r" && ~ro"o"t == "$r" ]] && echo same`, "same\n"},
		{`[[ ~"$x" == "$r" && ~${x}/q == "$r/q" && ~"root/x" == "$r/x" && ~"root":x == "$r:x" ]] && echo same`, "same\n"},
		{`[[ ~"/bar" == "$HOME/bar" && ~$e/x == "$HOME/x" ]] && echo same`, "same\n"},
		{`print -r -- ~"-" ~$m ~\+`, "/old /old /tmp\n"},
		{`print -r -- ~\/bar ~root\/x ~\- ~\-/x ~$y ~$q/x`, "~/bar ~root/x ~- ~-/x ~root/x ~/yy/x\n"},
		{`a=~"root" b=a:~"root"; [[ $a == "$r" && $b == "a:$r" ]] && case $r in ~"root") echo same;; esac`, "same\n"},
	} {
		out, errs, code := runKshScript(t, pre+tc.src+"\n")
		if out != tc.out || errs != "" || code != 0 {
			t.Errorf("%s\n got %q %q %d\nwant %q", tc.src, out, errs, code, tc.out)
		}
	}
}
