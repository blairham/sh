// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// What this shell does with the name an assignment prefix stands in front of
// at a builtin. Measured 2026-09-16 on bash 5.3.20 and bash 3.2.57, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin closed (#3437).

func promoteRun(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The prefix's entry is an exported one, so a builtin that reads the name's
// attributes back sees an exported name and a child the builtin starts is told
// about it. This shell is the only column in the panel that does either.
func TestAPrefixAtABuiltinIsExportedHere(t *testing.T) {
	if got := bash.Semantics().PrefixExportAtABuiltin; got != interp.PrefixExportAtABuiltinOn {
		t.Errorf("PrefixExportAtABuiltin = %v, want %v", got, interp.PrefixExportAtABuiltinOn)
	}
	out, st := promoteRun(t, `c=1
c=2 declare -p c
export z=1
z=2 declare -p z
declare -p c z`)
	want := `declare -x c="2"` + "\n" + `declare -x z="2"` + "\n" +
		`declare -- c="1"` + "\n" + `declare -x z="1"` + "\n"
	if out != want || st != 0 {
		t.Errorf("a prefix read back at a builtin = %q (status %d), want %q", out, st, want)
	}
}

// And the attribute is gone again once the command is over, which the two
// trailing rows above assert and which is what makes it the command's
// environment rather than a write to the shell.
func TestAPrefixAtABuiltinReachesTheChildHere(t *testing.T) {
	out, st := promoteRun(t, `v=1
v=9 eval 'env | grep "^v=" || echo none'
env | grep "^v=" || echo gone`)
	want := "v=9\ngone\n"
	if out != want || st != 0 {
		t.Errorf("a prefix before `eval` = %q (status %d), want %q", out, st, want)
	}
}

// A declaration that names the export or the readonly attribute over the very
// name its own prefix is holding keeps the prefix's value for the shell.
func TestADeclarationKeepsItsOwnPrefixHere(t *testing.T) {
	if got := bash.Semantics().DeclarationPromotesThePrefixEntry; got != interp.Yes {
		t.Errorf("DeclarationPromotesThePrefixEntry = %v, want Yes", got)
	}
	out, st := promoteRun(t, `b=7; b=8 readonly b;   declare -p b
d=7; d=8 export d;     declare -p d
y=1; y=2 typeset -r y; declare -p y
n=1; n=2 declare -ir n; declare -p n
g=7; g=8 export g=5;   declare -p g`)
	want := `declare -rx b="8"` + "\n" + `declare -x d="8"` + "\n" +
		`declare -rx y="2"` + "\n" + `declare -irx n="2"` + "\n" +
		`declare -x g="5"` + "\n"
	if out != want || st != 0 {
		t.Errorf("a declaration over its own prefix = %q (status %d), want %q", out, st, want)
	}
}

// And not every declaration: a bare one, some other attribute letter, a plus
// form taking the attribute off, and a declaration naming a *different* name
// all leave the shell's own value standing. These are the rows that say the
// axis is about the attribute rather than about the command's class — and the
// two special builtins beside them say it is not the persistence rule.
func TestADeclarationWithoutTheAttributeKeepsNothingHere(t *testing.T) {
	out, st := promoteRun(t, `x=1; x=2 declare x;     declare -p x
i=1; i=2 declare -i i;  declare -p i
t=1; t=2 declare +x t;  declare -p t
k=1; k=2 readonly j;    declare -p k
q=7; q=8 :;             echo "[$q]"
set -- a b
e=7; e=8 shift;         echo "[$e]"`)
	want := `declare -- x="1"` + "\n" + `declare -- i="1"` + "\n" +
		`declare -- t="1"` + "\n" + `declare -- k="1"` + "\n" +
		"[7]\n[7]\n"
	if out != want || st != 0 {
		t.Errorf("a declaration naming no attribute = %q (status %d), want %q", out, st, want)
	}
}

// A declaration that takes a *fresh scope* takes the entry with it, so the
// value is the local's and the caller's name is untouched — which is the half
// that makes the keeping above land in the scope the prefix is in rather than
// always in the globals.
func TestADeclarationInAFunctionTakesThePrefixIntoItsScopeHere(t *testing.T) {
	out, st := promoteRun(t, `b=8
f() { b=4 declare -r b; echo "in:[$b]"; }
f
echo "out:[$b]"; declare -p b
g() { local c; declare -p c; }
c=2 g
h() { local -r c; declare -p c; }
c=2 h
declare -p c 2>/dev/null || echo "c: gone"`)
	want := "in:[4]\nout:[8]\n" + `declare -- b="8"` + "\n" +
		`declare -x c="2"` + "\n" + `declare -rx c="2"` + "\n" + "c: gone\n"
	if out != want || st != 0 {
		t.Errorf("a declaration inside a function = %q (status %d), want %q", out, st, want)
	}
}
