// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// This shell's answer to which arm of an alternation a longest prefix trim
// takes: the longest of them, whichever order they were written in.
//
// Measured 2026-09-11 on bash 5.3.15 and 3.2.57 with `shopt -s extglob`, each
// row a `-c` of its own. It is here as well as in the zsh package because the
// axis is only visible as a disagreement: one of these two files passing on
// its own would say nothing about which reading this dialect has.
func TestALongestPrefixTrimTakesTheLongestArm(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s extglob; x=abc; printf '[%s]' "${x##@(a|ab)}"`, "[c]"},
		{`shopt -s extglob; x=abc; printf '[%s]' "${x##@(ab|a)}"`, "[c]"},
		{`shopt -s extglob; x=abc; printf '[%s]' "${x##@(a|ab|abc)}"`, "[]"},
		{`shopt -s extglob; x=abc; printf '[%s]' "${x#@(ab|a)}"`, "[bc]"},
		{`shopt -s extglob; x=abc; printf '[%s]' "${x%%@(bc|abc)}"`, "[]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
