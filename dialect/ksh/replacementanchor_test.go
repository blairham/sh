// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/ksh"
)

// The column that declines an anchor behind an **empty** pattern, and
// takes every other anchor there is: `v=abcabc` gives `Xbcabc` for
// `${v/#a/X}` and `abcabc` for `${v/#/X}`, where bash and zsh write the
// `X`. EmptyReplacementPattern's doc recorded this in prose from #1857
// and nothing read it until #3272.
func TestTheReplacementAnchorAnswers(t *testing.T) {
	s := ksh.Semantics()
	if got, want := s.ReplacementAnchors, interp.Yes; got != want {
		t.Errorf("ReplacementAnchors = %v, want %v", got, want)
	}
	if got, want := s.AnchoredEmptyReplacementPattern, interp.No; got != want {
		t.Errorf("AnchoredEmptyReplacementPattern = %v, want %v", got, want)
	}
}
