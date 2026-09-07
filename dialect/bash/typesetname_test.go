// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// An operand a declaration will not take, in this shell's one wording and at
// its one status — and *not* fatally, which is the half that separates it
// from the other two shells with the builtin.
//
// Measured 2026-09-07 against bash 5.3.15 and 3.2.57, which give the same
// sentence at the same status.
func TestADeclarationOperandThatIsNotAName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{
			`typeset ':'; echo "st=$?"; echo A`,
			"bash: line 1: typeset: `:': not a valid identifier\nst=1\nA\n",
		},
		{
			`typeset 1x; echo "st=$?"; echo A`,
			"bash: line 1: typeset: `1x': not a valid identifier\nst=1\nA\n",
		},
		{
			`declare ':'; echo "st=$?"; echo A`,
			"bash: line 1: declare: `:': not a valid identifier\nst=1\nA\n",
		},
		// The whole operand is quoted back, value and all, where zsh names
		// only the part before the `=`.
		{
			`typeset '1x=v'; echo "st=$?"`,
			"bash: line 1: typeset: `1x=v': not a valid identifier\nst=1\n",
		},
		// Every bad operand is reported and the well-formed ones are still
		// declared, which is this shell's alone: the other two stop at the
		// first.
		{
			`typeset ok=1 1x 2y; echo "st=$?"; echo "ok=$ok"`,
			"bash: line 1: typeset: `1x': not a valid identifier\n" +
				"bash: line 1: typeset: `2y': not a valid identifier\nst=1\nok=1\n",
		},
	} {
		out, st := runBash(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A subscripted operand is a name to a *declaration* here and not to
// `export`, which is the split that needed a third answer rather than the one
// `export` already had.
func TestADeclarationTakesASubscriptWhereExportDoesNot(t *testing.T) {
	dir := t.TempDir()
	// Not refused, which is the whole of what the axis decides here. What
	// the operand then *does* is a separate gap and is not asserted: real
	// bash creates the element and this shell declares nothing, which is
	// filed rather than fixed here — routing the check through must not
	// start refusing the line, and that is what this pins.
	out, st := runBash(t, dir, `typeset a[1]=v; echo "st=$?"`)
	if want := "st=0\n"; out != want || st != 0 {
		t.Errorf("typeset a[1]=v = %q (status %d), want %q at 0", out, st, want)
	}
	out, st = runBash(t, dir, `export b[1]=v; echo "st=$?"`)
	if want := "bash: line 1: export: `b[1]=v': not a valid identifier\nst=1\n"; out != want || st != 0 {
		t.Errorf("export b[1]=v = %q (status %d), want %q at 0", out, st, want)
	}
	if got := bash.Semantics().TypesetTakesASubscript; got != interp.Yes {
		t.Errorf("TypesetTakesASubscript = %v, want Yes", got)
	}
	if got := bash.Semantics().DeclarationTakesASubscript; got != interp.No {
		t.Errorf("DeclarationTakesASubscript = %v, want No", got)
	}
}

// The declaration builtins' entries in the bad-name table.
//
// Asserted because the *substrate's* default wording is this shell's sentence,
// so a missing entry here is invisible from the outside — every case above
// would still pass. The table is meant to be the whole answer for every
// builtin that reads it, and this is the only way to say so.
func TestTheDeclarationBadNameWordingsAreSet(t *testing.T) {
	d := bash.Diagnostics()
	const want = "%[1]s: `%[2]s': not a valid identifier"
	for _, b := range []string{"typeset", "declare", "export", "readonly", "local", "unset"} {
		if d.BuiltinBadName[b] != want {
			t.Errorf("BuiltinBadName[%q] = %q, want %q", b, d.BuiltinBadName[b], want)
		}
	}
	if !d.BuiltinBadNameKeepsValue {
		t.Error("BuiltinBadNameKeepsValue is false; this shell quotes the whole operand back")
	}
}
