// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/zsh"
)

// The backslash protects the character behind it **and** joins the set,
// which no other column does: `[\)]` matches `)` and a lone backslash,
// and `[\-z]` matches a dash, a z and a backslash while matching no y.
// Reachable only through `${~p}` here, a pattern written in the source
// having had its escapes spent by quote removal (#1407, #3271).
func TestBracketEscapeAnswer(t *testing.T) {
	if got, want := zsh.Semantics().BracketEscape, interp.BracketEscapeProtectsAndIsAMember; got != want {
		t.Errorf("BracketEscape = %v, want %v", got, want)
	}
}
