// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// TestTypeDropsALeadingDashWordAndAnswersBare is BusyBox ash 1.37.0's answer to
// a first `type` operand beginning with `-`, measured 2026-10-03 in the pinned
// image: the word is dropped whatever its letters, and every name after it is
// answered the way `command -v` answers — bare, the definition for an alias,
// silence for nothing and 127. Only the first word goes: a second `-t` is a
// name it cannot find.
func TestTypeDropsALeadingDashWordAndAnswersBare(t *testing.T) {
	if got := ash.Semantics().TypeLeadingDashWordAsksForTheBareAnswer; got != interp.Yes {
		t.Fatalf("TypeLeadingDashWordAsksForTheBareAnswer = %v, want yes", got)
	}
	for _, tc := range []struct{ src, want string }{
		{`f(){ :; }; type -t f; type -w cd; type -V if; echo "st=$?"`, "f\ncd\nif\nst=0\n"},
		{`type -- cd; echo "st=$?"`, "cd\nst=0\n"},
		{`type - cd; echo "st=$?"`, "cd\nst=0\n"},
		{`f(){ :; }; type -t -t f; echo "st=$?"`, "f\nst=127\n"},
		{`type -p nosuchzz_qq; echo "st=$?"`, "st=127\n"},
		{`alias pz=echo; type -w pz; echo "st=$?"`, "alias pz='echo'\nst=0\n"},
		// The control: with no leading dash it is the sentence.
		{`type cd; echo "st=$?"`, "cd is a shell builtin\nst=0\n"},
	} {
		if out, _ := run(t, tc.src); out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
