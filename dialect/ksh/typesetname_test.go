// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// An operand a declaration will not take, in this shell's wording and with
// its fatality: the script stops, so the line after the declaration is what
// the assertion is really about.
//
// Measured 2026-09-07 against ksh93u+ 2012-08-01.
func TestADeclarationOperandThatIsNotAName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset ':'; echo A`, "ksh: typeset: :: invalid variable name\n"},
		{`typeset 1x; echo A`, "ksh: typeset: 1x: invalid variable name\n"},
		// The whole operand, value attached, which is bash's rule and not
		// zsh's.
		{`typeset '1x=v'; echo A`, "ksh: typeset: 1x=v: invalid variable name\n"},
		// The third name renames itself in its own complaint: `integer 1x`
		// says `typeset`, where zsh's says `integer`.
		{`integer 1x; echo A`, "ksh: typeset: 1x: invalid variable name\n"},
	} {
		out, st := runKsh(t, dir, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", tc.src, out, st, tc.want)
		}
		if strings.Contains(out, "A") {
			t.Errorf("%s ran the line after the declaration: %q", tc.src, out)
		}
	}
}

// A subscripted operand is a name to both builtins here, which is the column
// that says the third answer is not simply bash's split written twice.
func TestADeclarationTakesASubscript(t *testing.T) {
	dir := t.TempDir()
	out, st := runKsh(t, dir, `typeset a[1]=v; echo "st=$?"`)
	if want := "st=0\n"; out != want || st != 0 {
		t.Errorf("typeset a[1]=v = %q (status %d), want %q at 0", out, st, want)
	}
	if got := ksh.Semantics().TypesetTakesASubscript; got != interp.Yes {
		t.Errorf("TypesetTakesASubscript = %v, want Yes", got)
	}
	if got := ksh.Diagnostics().BuiltinBadName["typeset"]; got != "%[1]s: %[2]s: invalid variable name" {
		t.Errorf(`BuiltinBadName["typeset"] = %q`, got)
	}
}
