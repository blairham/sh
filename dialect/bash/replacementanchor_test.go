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
}
