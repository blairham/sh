// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A declaration inside a function meeting a name this shell has frozen.
// Measured 2026-09-06 on bash 5.3.15, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, over a script file (#1159).

// The shadow is refused here and zsh takes it, which is the axis. Three
// things follow from the refusal and all three were wrong: the builtin is
// named, the builtin reports 1, and the **function runs on**.
func TestADeclarationWillNotShadowAFrozenNameHere(t *testing.T) {
	if got := bash.Semantics().DeclarationMayShadowAReadonly; got != interp.No {
		t.Errorf("DeclarationMayShadowAReadonly = %v, want No", got)
	}
	if !bash.Diagnostics().ReadonlyRefusalNamesBuiltin["local"] {
		t.Error("ReadonlyRefusalNamesBuiltin has no `local` entry, so the refusal " +
			"cannot name the word the script wrote")
	}
	out, st := runBash(t, t.TempDir(), `typeset -r x=1
f() { local x=2; echo "lst=$?"; echo "in=[$x]"; echo running; }
f; echo "fst=$? out=[$x]"
g() { typeset x; echo "tst=$? tin=[$x]"; }
g
k() { local x=2 || echo or-fired; echo "kin=[$x]"; }
k
j() { local y=1 x=5 z=2; echo "jin y=[$y] x=[$x] z=[$z]"; }
j
echo after`)
	// The whole of it, refusals interleaved where they fall, which is
	// byte-for-byte what bash 5.3.15 answers this script.
	want := "bash: line 2: local: x: readonly variable\n" +
		"lst=1\nin=[1]\nrunning\nfst=0 out=[1]\n" +
		"bash: line 4: typeset: x: readonly variable\n" +
		"tst=1 tin=[1]\n" +
		"bash: line 6: local: x: readonly variable\n" +
		"or-fired\nkin=[1]\n" +
		"bash: line 8: local: x: readonly variable\n" +
		"jin y=[1] x=[1] z=[2]\nafter\n"
	if out != want || st != 0 {
		t.Errorf("declarations over a frozen name = %q (status %d), want %q", out, st, want)
	}
	// Four declarations of the frozen name and four refusals, each naming
	// the word the script wrote — the valueless `typeset x` among them,
	// which met no check at all before.
	if n := strings.Count(out, "readonly variable"); n != 4 {
		t.Errorf("out = %q has %d refusals, want one per declaration", out, n)
	}
}
