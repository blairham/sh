// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// This shell's answer to what an empty pattern matches: only a value with
// nothing in it.
//
// Measured 2026-09-12 on ksh93u+ from a script file. The middle column of the
// three, and the one that says the axis needed three answers rather than two
// — bash declines the pattern whatever the value, and zsh takes it wherever
// any empty-matching pattern would (#1857).
func TestAnEmptyPatternMatchesOnlyAnEmptyValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abc; printf '[%s]' "${v///X}"`, "[abc]"},
		{`v=abc; p=; printf '[%s]' "${v//$p/X}"`, "[abc]"},
		{`v=abc; p=; printf '[%s]' "${v/$p/X}"`, "[abc]"},
		// The row that parts this shell from bash, which leaves an empty
		// value alone as well.
		{`v=; printf '[%s]' "${v///X}"`, "[X]"},
		// The control: an empty value does not produce the replacement on its
		// own, so the row above is a match and not an artifact.
		{`v=; printf '[%s]' "${v//x/X}"`, "[]"},
		// Not a refusal of empty matches: a pattern with bytes in it that
		// matches only the empty string fires at every position, and at the
		// end of the value too — which is this shell's other answer below.
		{`v=abc; printf '[%s]' "${v//@(|)/<>}"`, "[<>a<>b<>c<>]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// And which empty match this shell declines: the one sitting where the match
// before it ended, which is the classic global-replace rule. The end of the
// value is a position like any other.
func TestTheEmptyMatchDeclinedIsTheOneAfterAMatch(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The discriminating row: no `<>` between `b` and `c`, where bash and
		// zsh have two, and one after `c`, where they have none.
		{`v=abc; printf '[%s]' "${v//@(b|)/<>}"`, "[<>a<>c<>]"},
		{`v=abc; printf '[%s]' "${v//@(x|)/<>}"`, "[<>a<>b<>c<>]"},
		{`v=abc; printf '[%s]' "${v//@(a|)/<>}"`, "[<>b<>c<>]"},
		// The control both readings answer alike: the letter matches at the
		// last unit, so the scan ends there whichever rule is in force.
		{`v=abc; printf '[%s]' "${v//@(c|)/<>}"`, "[<>a<>b<>]"},
		{`v=abc; printf '[%s]' "${v//*/<>}"`, "[<>]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
