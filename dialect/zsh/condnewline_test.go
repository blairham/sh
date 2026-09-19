// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell alone lets a condition term's first word be the last thing on
// its line, with whatever decides the term written on the next.
//
// Measured 2026-09-18, script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`: `[[ y` with the `]]`, a `&&`, a `||` or a `)` on the following
// line all run here and are all refused by bash 5.3.20, bash 3.2 and ksh93u+,
// each naming the newline (#3627).
func TestATermsFirstWordMayEndItsLineHere(t *testing.T) {
	d := zsh.Dialect()
	if !d.ConditionNewlineMayFollowATermsFirstWord {
		t.Error("ConditionNewlineMayFollowATermsFirstWord is false, want true")
	}
	for _, src := range []string{
		"[[ y\n]]\n",
		"[[ y\n&& -n z ]]\n",
		"[[ y\n|| -n z ]]\n",
		"[[ ( y\n) ]]\n",
		"[[ -n x && y\n]]\n",
	} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
	// And not around a binary operator, which this shell refuses too: the
	// flag is about a term with no verdict, not about newlines.
	for _, src := range []string{"[[ y\n== z ]]\n", "[[ y ==\nz ]]\n"} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err == nil {
			t.Errorf("%q parsed, want a refusal", src)
		}
	}
}
