// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// What this shell does with the name an assignment prefix stands in front of
// at a builtin. Measured 2026-09-16 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, from a script file with stdin closed (#3437).

// The attribute is left exactly where it was — neither gained nor taken off —
// which is the third answer and the one a two-valued axis could not express.
func TestAPrefixAtABuiltinLeavesTheAttributeAloneHere(t *testing.T) {
	if got := zsh.Semantics().PrefixExportAtABuiltin; got != interp.PrefixExportAtABuiltinUnchanged {
		t.Errorf("PrefixExportAtABuiltin = %v, want %v", got, interp.PrefixExportAtABuiltinUnchanged)
	}
	out, st := runZsh(t, t.TempDir(), `c=1
c=2 typeset -p c
export z=1
z=2 typeset -p z
typeset -p c z`)
	want := "typeset c=2\nexport z=2\ntypeset c=1\nexport z=1\n"
	if out != want || st != 0 {
		t.Errorf("a prefix read back at a builtin = %q (status %d), want %q", out, st, want)
	}
}

// And a declaration keeps nothing: the temporary the prefix made is what the
// attribute went on, and it leaves with the command — value and attribute
// both, which is what the unfrozen `b` and `y` below say.
func TestADeclarationKeepsNoneOfItsPrefixHere(t *testing.T) {
	if got := zsh.Semantics().DeclarationPromotesThePrefixEntry; got != interp.No {
		t.Errorf("DeclarationPromotesThePrefixEntry = %v, want No", got)
	}
	out, st := runZsh(t, t.TempDir(), `b=7; b=8 readonly b;   typeset -p b
d=7; d=8 export d;     typeset -p d
y=1; y=2 typeset -r y; typeset -p y
b=70; y=20; echo "writable [$b] [$y]"`)
	want := "typeset b=7\ntypeset d=7\ntypeset y=1\nwritable [70] [20]\n"
	if out != want || st != 0 {
		t.Errorf("a declaration over its own prefix = %q (status %d), want %q", out, st, want)
	}
}
