// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// A `local` over a name this shell has frozen is refused, and the refusal
// ends the script. Measured 2026-09-07 on dash, `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and HISTFILE, over a script file.
//
// The axis had been written down as one this shell could not ask, for want of
// a `typeset -r` to freeze with — but `readonly` is POSIX and `local` is the
// one declaration here, so both words the question needs do exist. It
// answers, and it answers `No` (#1168).
func TestADeclarationWillNotShadowAFrozenNameHere(t *testing.T) {
	if got := dash.Semantics().DeclarationMayShadowAReadonly; got != interp.No {
		t.Errorf("DeclarationMayShadowAReadonly = %v, want No", got)
	}
	if !dash.Diagnostics().ReadonlyRefusalNamesBuiltin["local"] {
		t.Error("ReadonlyRefusalNamesBuiltin has no `local` entry, so the refusal " +
			"loses the builtin's name — this shell names all three of the " +
			"declarations it has")
	}
	out, st := runDash(t, t.TempDir(), `readonly x=1
f() { local x=2; echo "in=[$x]"; }
f
echo never`)
	if !strings.Contains(out, "local: x: is read only") {
		t.Errorf("`local` over a frozen name = %q, want the refusal with `local` "+
			"named", out)
	}
	if strings.Contains(out, "never") || strings.Contains(out, "in=[") || st != 2 {
		t.Errorf("out = %q (status %d), want the script to end and exit 2 — a "+
			"declaration's refusal is fatal here", out, st)
	}
}

// And an ordinary `local`, over a name nothing froze, is untouched by any of
// it: the question is asked only where a declaration meets a freeze, so every
// function in every script must not reach it.
func TestAnOrdinaryLocalIsUnaffectedHere(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `x=1
f() { local x=2; echo "in=[$x]"; }
f
echo "out=[$x]"`)
	want := "in=[2]\nout=[1]\n"
	if out != want || st != 0 {
		t.Errorf("an ordinary local = %q (status %d), want %q", out, st, want)
	}
}
