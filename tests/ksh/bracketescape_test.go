// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/ksh"
)

// The value five of the seven columns share: the backslash is spent on
// the character behind it and puts nothing of its own in the set, so
// `[\)]` is `)` alone and `[a\-z]` is the three members a, `-` and z.
// zsh adds the backslash as well and BusyBox ash protects nothing and
// reads the dash as the range operator anyway, which is why this is a
// three-valued axis rather than the bool it was (#3271).
func TestBracketEscapeAnswer(t *testing.T) {
	if got, want := ksh.Semantics().BracketEscape, interp.BracketEscapeProtectsTheMember; got != want {
		t.Errorf("BracketEscape = %v, want %v", got, want)
	}
}
