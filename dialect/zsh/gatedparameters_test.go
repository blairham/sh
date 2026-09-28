// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"sort"
	"strings"
	"testing"
)

// The seven parameters that do not exist until their module is loaded.
//
// The third state a module parameter can be in. Thirty-eight are registered
// at startup and wait for the script's first reference — that is the deferral
// and deferredparameters_test.go grades it. These seven are not registered at
// all, because their modules declare no autoloadable parameter, and this
// shell had them from startup.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`, as the first statement so that nothing has
// referred to any of them.
//
// **Every row is a pair**, which is the whole design of this rather than
// thoroughness: the same question is asked before the load and after it, and
// only a row that moves between the two says the state exists. A grid taken
// only after a load agrees with a shell that gates nothing.
func TestAModuleParameterDoesNotExistUntilItsModuleLoads(t *testing.T) {
	for _, tc := range []struct{ name, module, typeWord, listed string }{
		// Not readonly, where every other association a module brings here
		// is: the freeze on `$langinfo` is on its elements and the
		// parameter carries no attribute at all. Re-measured 2026-09-28
		// and #4996 moved the row; see langinfo.go for the grid.
		{"langinfo", "zsh/langinfo", "association-hide-hideval-special", "typeset -A langinfo"},
		{"mapfile", "zsh/mapfile", "association-hide-hideval-special", "typeset -A mapfile"},
		{"sysparams", "zsh/system", "association-readonly-hide-hideval-special", "typeset -Ar sysparams"},
		{"errnos", "zsh/system", "array-readonly-hide-hideval-special", "typeset -ar errnos"},
		{"epochtime", "zsh/datetime", "array-readonly-hide-hideval-special", "typeset -ar epochtime"},
		// The two the issue asked to be measured beside `epochtime` rather
		// than assumed to follow it. They follow it.
		{"EPOCHSECONDS", "zsh/datetime", "integer-readonly-hide-hideval-special", "typeset -ir EPOCHSECONDS"},
		{"EPOCHREALTIME", "zsh/datetime", "float-readonly-hide-hideval-special", "typeset -Fr EPOCHREALTIME"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Before: the three spellings an invented name answers.
			before, st := runZsh(t, t.TempDir(),
				`print -r -- "plus=${+`+tc.name+`} t=[${(t)`+tc.name+`}]"
typeset -p `+tc.name+`
print -r -- "st=$?"`)
			want := "plus=0 t=[]\nzsh:typeset:2: no such variable: " + tc.name + "\nst=1\n"
			if before != want || st != 0 {
				t.Errorf("$%s before the load = %q (status %d), want %q", tc.name, before, st, want)
			}
			// After: all three move, which is what keeps the fix from being
			// "delete the registration".
			after, st := runZsh(t, t.TempDir(),
				`zmodload `+tc.module+`
print -r -- "plus=${+`+tc.name+`} t=[${(t)`+tc.name+`}]"
typeset -p `+tc.name+`
print -r -- "st=$?"`)
			want = "plus=1 t=[" + tc.typeWord + "]\n" + tc.listed + "\nst=0\n"
			if after != want || st != 0 {
				t.Errorf("$%s after the load = %q (status %d), want %q", tc.name, after, st, want)
			}
		})
	}
}

// And the control on either side, in one run: a name nothing registers
// answers exactly the same three ways as a gated one, and a name that is
// merely **deferred** answers a third way.
//
// This is what says the gate is modeling "not there" rather than "hidden".
// `funcstack` is registered and waiting, so `typeset -p` is silent at 0 for
// it where `langinfo` is `no such variable` at 1 and `neverheardof` is the
// same — three states, told apart in one shell.
func TestTheThreeStatesAreToldApartInOneShell(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -p funcstack; print -r -- "deferred=$?"
typeset -p langinfo
print -r -- "gated=$?"
typeset -p neverheardof
print -r -- "invented=$?"`)
	want := "deferred=0\n" +
		"zsh:typeset:2: no such variable: langinfo\ngated=1\n" +
		"zsh:typeset:4: no such variable: neverheardof\ninvented=1\n"
	if out != want || st != 0 {
		t.Errorf("the three states = %q (status %d), want %q", out, st, want)
	}
}

// The module still loads, which is the row that would have been silent: the
// gate `zmodload` opens is "does this shell have every feature the module
// names", and a parameter that is not registered yet is exactly what that
// test reads as missing.
//
// Without zshGatedParameterModule the load refused with `langinfo is not
// implemented yet` — the module rule firing at the name the module is about
// to bring — and every row in the test above would have failed at the load
// rather than at the parameter, which is a different bug wearing this one's
// output.
func TestAGatedParametersModuleStillLoads(t *testing.T) {
	for _, module := range []string{"zsh/langinfo", "zsh/mapfile", "zsh/system", "zsh/datetime"} {
		t.Run(module, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), `zmodload `+module+` 2>&1
print -r -- "st=$?"
zmodload -e `+module+`
print -r -- "e=$?"`)
			if want := "st=0\ne=0\n"; out != want || st != 0 {
				t.Errorf("zmodload %s = %q (status %d), want %q", module, out, st, want)
			}
		})
	}
}

// The two tables in gatedparameters.go are checked against each other.
//
// A module named in the roster with nothing to run is a module whose
// parameters would never arrive, and it is **silent**: `${+langinfo}` would
// simply stay 0 and the `zmodload` would still report success, which is the
// shape of failure the whole module rule exists to avoid.
func TestEveryGatedModuleHasAnInstaller(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/langinfo zsh/mapfile zsh/system zsh/datetime
print -r -- "${+langinfo}${+mapfile}${+sysparams}${+errnos}${+epochtime}${+EPOCHSECONDS}${+EPOCHREALTIME}"`)
	// Seven ones: every name the roster promises is there after the loads it
	// promises them behind. The same line before the loads is seven zeros,
	// which is the pair TestAModuleParameterDoesNotExistUntilItsModuleLoads
	// asks name by name.
	if want := strings.Repeat("1", 7) + "\n"; out != want || st != 0 {
		t.Errorf("after loading every gated module = %q (status %d), want %q", out, st, want)
	}
	before, st := runZsh(t, t.TempDir(),
		`print -r -- "${+langinfo}${+mapfile}${+sysparams}${+errnos}${+epochtime}${+EPOCHSECONDS}${+EPOCHREALTIME}"`)
	if want := strings.Repeat("0", 7) + "\n"; before != want || st != 0 {
		t.Errorf("before any load = %q (status %d), want %q", before, st, want)
	}
}

// And nothing else moved: the thirty-eight deferred names are still deferred
// and still arrive on a first reference, which is the neighboring mechanism
// this one is most likely to have been folded into.
//
// The roster is the assertion. A gated name in the deferred list would have
// made `typeset -p langinfo` silent at 0, which is neither shell's answer,
// and a deferred name in the gated list would have made `typeset -p
// funcstack` a refusal.
func TestTheGatedAndDeferredRostersDoNotOverlap(t *testing.T) {
	gated := []string{"langinfo", "mapfile", "sysparams", "errnos", "epochtime", "EPOCHSECONDS", "EPOCHREALTIME"}
	sort.Strings(gated)
	for _, name := range gated {
		if gatedModuleOf(name) == "" {
			t.Errorf("%s is gated and the tests' own lookup does not say so", name)
		}
	}
	// The other direction: a name the deferral roster holds must not be
	// gated, and `funcstack` is the one every other test in that file uses.
	if got := gatedModuleOf("funcstack"); got != "" {
		t.Errorf("funcstack is deferred and gatedModuleOf says %q", got)
	}
}
