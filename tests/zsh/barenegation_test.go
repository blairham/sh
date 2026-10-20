// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// This shell takes a bare `!` where a *list* ends, and not before a `&` —
// which is the same boundary OpenEndedAndOr has here, and the row that tells
// this reach from bash's. Measured 2026-09-12 on zsh 5.9.2.
func TestABareNegationStandsWhereAListEndsHere(t *testing.T) {
	if got := zsh.Dialect().BareNegationReach; got != syntax.BareNegationWhereAListEnds {
		t.Errorf("reach = %v, want BareNegationWhereAListEnds", got)
	}
	for _, tc := range []struct{ src, want string }{
		{`true; !; echo "st=$?"`, "st=1"},
		{`false; !; echo "st=$?"`, "st=1"},
		{`{ ! ; echo "st=$?"; }`, "st=1"},
		{`( ! ); echo "st=$?"`, "st=1"},
		{`case x in x) ! ;; esac; echo "st=$?"`, "st=1"},
		{`! || echo two`, "two"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
	for _, src := range []string{
		// A `&` is not a place a list ends here, which bash's reach takes
		// and this one does not.
		"! & echo x\n",
		// And a second `!` is refused rather than toggling, so the two
		// questions really are separate: this shell has the reach and not
		// the toggle.
		"! ! true\n",
		"! !\n",
		"! | cat\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
	if zsh.Dialect().RepeatedNegationToggles {
		t.Error("this shell refuses a second `!` rather than toggling")
	}
}
