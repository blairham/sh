// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell's answer to what an empty pattern matches: everywhere an ordinary
// pattern matching the empty string would, and it is alone in the panel.
//
// Measured 2026-09-12 on zsh 5.9.2 from a script file (#1857).
func TestAnEmptyPatternMatchesEveryPosition(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// bash and ksh93 both leave the value alone here.
		{`v=abc; printf '[%s]' "${v///X}"`, "[XaXbXc]"},
		{`v=abc; p=; printf '[%s]' "${v//$p/X}"`, "[XaXbXc]"},
		{`v=abc; p=; printf '[%s]' "${v/$p/X}"`, "[Xabc]"},
		{`v=; printf '[%s]' "${v///X}"`, "[X]"},
		// The end of the value is not one of the positions, which is the
		// general empty-match rule rather than anything about this pattern:
		// the result is three replacements for three units and not four.
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//(#e)/X}"`, "[abcX]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// And which empty match this shell declines: the one at the end of the value,
// which is bash's reading and not ksh93's.
func TestTheEmptyMatchDeclinedIsTheOneAtTheEnd(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The discriminating row. ksh93 gives `[<>a<>c<>]` for the same
		// pattern.
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//(b|)/<>}"`, "[<>a<><>c]"},
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//(x|)/<>}"`, "[<>a<>b<>c]"},
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//(a|)/<>}"`, "[<><>b<>c]"},
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//(c|)/<>}"`, "[<>a<>b<>]"},
		// The end is scanned when it was reached by copying a character
		// nothing matched, which is what says the rule is about the step and
		// not about the position.
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//x#/-}"`, "[-a-b-c]"},
		{`setopt extendedglob; v=abc; printf '[%s]' "${v//(#e)/-}"`, "[abc-]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
