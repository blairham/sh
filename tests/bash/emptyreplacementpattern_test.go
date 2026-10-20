// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// This shell's answers to the two empty-match axes a span replacement asks.
//
// Measured 2026-09-12 on bash 5.3.15, on 3.2.57 and under argv[0] `sh`, from
// a script file. It is here as well as in the ksh and zsh packages because
// each axis is only visible as a disagreement: one of these files passing on
// its own would say nothing about which reading this dialect has (#1857,
// #1859).
func TestAnEmptyPatternReplacesNowhere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abc; printf '[%s]' "${v///X}"`, "[abc]"},
		{`v=abc; p=; printf '[%s]' "${v//$p/X}"`, "[abc]"},
		{`v=abc; p=; printf '[%s]' "${v/$p/X}"`, "[abc]"},
		// The row that parts this shell from ksh93, which takes the pattern
		// where there is nothing to scan.
		{`v=; printf '[%s]' "${v///X}"`, "[]"},
		// And the control that says the row above is a match declined rather
		// than an empty value: a pattern this value cannot match is the same
		// empty result, and one it can match is not.
		{`v=; printf '[%s]' "${v//x/X}"`, "[]"},
		{`v=; printf '[%s]' "${v//*/X}"`, "[X]"},
		// Not a refusal of empty matches in general: a pattern with bytes in
		// it that matches only the empty string fires at every position.
		{`shopt -s extglob; v=abc; printf '[%s]' "${v//@(|)/<>}"`, "[<>a<>b<>c]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// And which empty match this shell declines: the one at the end of the value,
// taking the one that sits where the match before it ended.
func TestTheEmptyMatchDeclinedIsTheOneAtTheEnd(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The discriminating row: two `<>` between `a` and `c`, and none
		// after `c`. ksh93 gives `[<>a<>c<>]` for the same source.
		{`shopt -s extglob; v=abc; printf '[%s]' "${v//@(b|)/<>}"`, "[<>a<><>c]"},
		{`shopt -s extglob; v=abc; printf '[%s]' "${v//@(x|)/<>}"`, "[<>a<>b<>c]"},
		{`shopt -s extglob; v=abc; printf '[%s]' "${v//@(a|)/<>}"`, "[<><>b<>c]"},
		// The control both readings answer alike, so it says the rule the
		// whole panel agrees on is untouched.
		{`shopt -s extglob; v=abc; printf '[%s]' "${v//@(c|)/<>}"`, "[<>a<>b<>]"},
		{`v=abc; printf '[%s]' "${v//*/<>}"`, "[<>]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
