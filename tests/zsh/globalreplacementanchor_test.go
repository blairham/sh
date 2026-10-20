// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/zsh"
)

// A `#` or a `%` after the **global** `//` is an anchor here and is the
// pattern's own first character everywhere else (#3307).
//
// Measured 2026-09-16 against zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, stdin from /dev/null, in a fresh directory.
// Every row below was taken against bash 5.3.20, bash-as-`sh`, bash 3.2.57
// and ksh93u+ 2012-08-01 in the same run, and this shell is alone in each.
func TestTheGlobalSpellingTakesAnAnchor(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// The control, and it is what makes every row below about the
			// *global* spelling rather than about the anchor: every column
			// that has anchors at all reads this one.
			"the single spelling is unanimous",
			`v=abcabc; printf '%s' "${v/#a/X}"`,
			"Xbcabc",
		},
		{
			// The row the issue was filed on. `abcabc` in the other four.
			"and the global spelling anchors here",
			`v=abcabc; printf '%s' "${v//#a/X}"`,
			"Xbcabc",
		},
		{
			"the end anchor too",
			`v=abcabc; printf '%s' "${v//%c/Y}"`,
			"abcabY",
		},
		{
			// **The discriminating probe.** `abcabc` cannot settle it on
			// its own: it is the answer both when the anchor is honored and
			// `a` does not start the value, and when the pattern is `#a`
			// and is not in the value. A value that *holds* the anchor
			// character parts the two — this shell leaves it alone because
			// it anchored, and the others replace because they did not.
			"a value holding the character tells the two readings apart",
			`w='x#ay%bz'; printf '%s' "${w//#a/Q}"`,
			"x#ay%bz",
		},
		{
			"and at the other end",
			`w='x#ay%bz'; printf '%s' "${w//%b/Q}"`,
			"x#ay%bz",
		},
		{
			// One byte more of the anchor character is a *pattern*
			// character again, which is what says the parser reads exactly
			// one: `##a` is the anchor and then the pattern `#a`, which
			// does not start `abcabc`.
			"a doubled character is the anchor and then a pattern byte",
			`v=abcabc; printf '%s' "${v//##a/X}"`,
			"abcabc",
		},
		{
			// The anchor with nothing behind it, which is the empty pattern
			// at that end — the same answer the single spelling gives here.
			"an anchor alone matches the empty string at that end",
			`v=abcabc; printf '%s' "${v//#/X}"`,
			"Xabcabc",
		},
		{
			"and at the end of the value",
			`v=abcabc; printf '%s' "${v//%/Y}"`,
			"abcabcY",
		},
		{
			// A real pattern behind the anchor, so the row is not about the
			// one-character case: anchored at the start, `a*` takes the
			// whole value.
			"a pattern behind the anchor is still a pattern",
			`v=abcabc; printf '%s' "${v//#a*/X}"`,
			"X",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The value behind the rows. Beside ReplacementAnchors rather than folded
// into it, because they are two questions: the other four columns answer the
// first yes and this one no.
func TestTheGlobalReplacementAnchorAnswer(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.GlobalReplacementAnchors, interp.Yes; got != want {
		t.Errorf("GlobalReplacementAnchors = %v, want %v", got, want)
	}
	if got, want := s.ReplacementAnchors, interp.Yes; got != want {
		t.Errorf("ReplacementAnchors = %v, want %v", got, want)
	}
}
