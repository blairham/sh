// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/bash"
)

// bash writes the `X` at either end: `v=abcabc` gives `Xabcabc` for
// `${v/#/X}` and `abcabcX` for `${v/%/X}`. ksh93 is the column that
// declines an anchor behind an empty pattern (#3272).
func TestTheReplacementAnchorAnswers(t *testing.T) {
	s := bash.Semantics()
	if got, want := s.ReplacementAnchors, interp.Yes; got != want {
		t.Errorf("ReplacementAnchors = %v, want %v", got, want)
	}
	if got, want := s.AnchoredEmptyReplacementPattern, interp.Yes; got != want {
		t.Errorf("AnchoredEmptyReplacementPattern = %v, want %v", got, want)
	}
	// And the anchor is read after a single `/` only. zsh is the one column
	// that reads one after the global `//` as well (#3307).
	if got, want := s.GlobalReplacementAnchors, interp.No; got != want {
		t.Errorf("GlobalReplacementAnchors = %v, want %v", got, want)
	}
}

// The behavior behind that value, and the row that is not `abcabc`: a value
// holding the anchor character is what tells "the anchor was honored and `a`
// does not start it" from "the pattern is `#a` and is not there". Measured
// 2026-09-16 on bash 5.3.20 and bash 3.2.57 alike.
func TestTheGlobalSpellingReadsNoAnchor(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the single spelling anchors", `v=abcabc; printf '%s' "${v/#a/X}"`, "Xbcabc"},
		{"the global spelling does not", `v=abcabc; printf '%s' "${v//#a/X}"`, "abcabc"},
		{"and the pattern is found where it stands", `w='x#ay%bz'; printf '%s' "${w//#a/Q}"`, "xQy%bz"},
		{"at the other end too", `w='x#ay%bz'; printf '%s' "${w//%b/Q}"`, "x#ayQz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, dir, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
