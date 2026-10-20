// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/zsh"
)

// zsh answers bash's way here rather than ksh93's: `v=abcabc` gives
// `Xabcabc` for `${v/#/X}` and `abcabcX` for `${v/%/X}` (#3272).
func TestTheReplacementAnchorAnswers(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.ReplacementAnchors, interp.Yes; got != want {
		t.Errorf("ReplacementAnchors = %v, want %v", got, want)
	}
	if got, want := s.AnchoredEmptyReplacementPattern, interp.Yes; got != want {
		t.Errorf("AnchoredEmptyReplacementPattern = %v, want %v", got, want)
	}
}
