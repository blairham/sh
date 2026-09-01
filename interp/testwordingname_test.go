// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

// A `test` diagnostic is written by the builtin under whichever of its two
// names was typed, so no wording may spell one of them itself. Six strings per
// dialect, each written separately, and the ones a corpus case does not reach
// are exactly the ones that would go unnoticed — so the claim is made about
// all of them at once rather than about the handful that are easy to run.
//
// The name slot is `%[2]s`; zsh uses none, because it puts the name in the
// location instead. What is forbidden is a *literal* name.
func TestNoTestWordingSpellsItsOwnName(t *testing.T) {
	for _, d := range []struct {
		dialect string
		diag    Diagnostics
	}{
		{"bash", bash.Diagnostics()},
		{"dash", dash.Diagnostics()},
		{"ksh93", ksh.Diagnostics()},
		{"zsh", zsh.Diagnostics()},
	} {
		for _, w := range []struct{ field, text string }{
			{"TestUnaryExpected", d.diag.TestUnaryExpected},
			{"TestBinaryExpected", d.diag.TestBinaryExpected},
			{"TestIntegerExpected", d.diag.TestIntegerExpected},
			{"TestTooManyArguments", d.diag.TestTooManyArguments},
			{"TestOperandExpected", d.diag.TestOperandExpected},
		} {
			if strings.HasPrefix(w.text, "test:") || strings.HasPrefix(w.text, "[:") {
				t.Errorf("%s %s = %q: names a builtin that may have been called by its other name",
					d.dialect, w.field, w.text)
			}
		}
		// The one wording that may spell a bracket, because only one of the
		// two names can reach it: nothing looks for a closing bracket unless
		// one opened. Which bracket it names is the dialect's business —
		// three blame the missing `]`, and bash names the command as well.
		if got := d.diag.TestMissingBracket; got != "" && !strings.ContainsAny(got, "[]") {
			t.Errorf("%s TestMissingBracket = %q: want a bracket in it", d.dialect, got)
		}
	}
}
