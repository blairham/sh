// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestAFailedExpansionInADeclarationOrCaseLeavesTheStatus pins that a failed
// expansion in a declaration's words, from its first assignment on, or in a
// `case`'s subject or patterns, stops the script with `$?` as it was — 0 in a
// `case` — where everywhere else it sets 1 (#5657). Through the binary,
// because a tilde's user lookup is the front end's hook. Measured 2026-10-03
// on zsh 5.9.2 under -f.
func TestAFailedExpansionInADeclarationOrCaseLeavesTheStatus(t *testing.T) {
	const div = "zsh:1: division by zero\n"
	for _, tc := range []struct {
		src, out, errs string
		status         int
	}{
		{`local x=$((1/0)); print after`, "", div, 0},
		{`false; local x=$((1/0))`, "", div, 1},
		{`local y=$(false) x=$((1/0))`, "", div, 1},
		{`local x=1 $((1/0))`, "", div, 0},
		{`local $((1/0))`, "", div, 1},
		{`export x=~nosuch5657`, "", "zsh:1: no such user or named directory: nosuch5657\n", 0},
		{`local -a x=(*nonex5657)`, "", "zsh:1: no matches found: *nonex5657\n", 0},
		{`local x=~[nodyn]`, "", "zsh:1: no directory expansion: ~[nodyn]\n", 0},
		{`case x in $((1/0))) ;; esac`, "", div, 0},
		{`false; case $(exit 3) in $((1/0))) ;; esac`, "", div, 0},
		{`case ~nosuch5657 in x) ;; esac`, "", "zsh:1: no such user or named directory: nosuch5657\n", 0},
		// A failure of its own sets 1 there too, and so does every other
		// place.
		{`case x in ${nosuch?m}) ;; esac`, "", "zsh:1: nosuch: m\n", 1},
		{`print $((1/0))`, "", div, 1},
		{`x=$((1/0))`, "", div, 1},
		// The always half sees `$?` as it was, and the construct still ends
		// with the fatal status.
		{`{ local x=$((1/0)); } always { print al $?; }`, "al 0\n", div, 1},
	} {
		out, errs, status := runZsh(t, "-fc", tc.src)
		if out != tc.out || errs != tc.errs || status != tc.status {
			t.Errorf("%s\n got %q %q %d\nwant %q %q %d", tc.src, out, errs, status, tc.out, tc.errs, tc.status)
		}
	}
}
