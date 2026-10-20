// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// This shell takes a bare `!` at either place — a statement terminator, and
// wherever a list ends — which is the union of the other two answers rather
// than a third rule. Measured 2026-09-12 on ksh93u+.
func TestABareNegationStandsAtEitherPlaceHere(t *testing.T) {
	if got := ksh.Dialect().BareNegationReach; got != syntax.BareNegationAtEitherPlace {
		t.Errorf("reach = %v, want BareNegationAtEitherPlace", got)
	}
	for _, tc := range []struct{ src, want string }{
		{`true; !; echo "st=$?"`, "st=1"},
		{`false; !; echo "st=$?"`, "st=1"},
		{`{ ! ; echo "st=$?"; }`, "st=1"},
		{`( ! ); echo "st=$?"`, "st=1"},
		{`if :; then !; fi; echo "st=$?"`, "st=1"},
		{`case x in x) ! ;; esac; echo "st=$?"`, "st=1"},
		{`! || echo two`, "two"},
		// And the toggle, as in bash.
		{`! ! true; echo "st=$?"`, "st=0"},
		{`! !; echo "st=$?"`, "st=0"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
	// A bar is refused here as everywhere.
	if _, err := syntax.Parse("! | cat\n", ksh.Dialect()); err == nil {
		t.Error("`! | cat` parsed, want a syntax error")
	}
}
