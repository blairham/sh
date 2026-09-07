// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `set -A name value …`, which this shell has and bash and dash do not.
// Measured 2026-09-06 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with
// a scratch HOME, ZDOTDIR and HISTFILE, over a script file.

// The assignment, the base it reads back at, and the plus form.
//
//	set -A a x y z          st=0 [x y z] n=3, a[0] is x
//	set -A a p              st=0 [p] n=1 — it replaces
//	set -A b 1 2 3 4 5; set +A b Q R   [Q R 3 4 5]
func TestSetArrayAssignsHere(t *testing.T) {
	if got := ksh.Semantics().SetArrayLetter; got != interp.Yes {
		t.Errorf("SetArrayLetter = %v, want Yes", got)
	}
	out, st := runKsh(t, t.TempDir(), `set -A a x y z; echo "A st=$? [${a[@]}] n=${#a[@]}"
echo "B zero=[${a[0]}] one=[${a[1]}]"
set -A a p; echo "C [${a[@]}] n=${#a[@]}"
set -A b 1 2 3 4 5
set +A b Q R; echo "D [${b[@]}] n=${#b[@]}"
h=hh; set -A $h m n; echo "E [${hh[@]}] n=${#hh[@]}"`)
	want := "A st=0 [x y z] n=3\nB zero=[x] one=[y]\nC [p] n=1\n" +
		"D [Q R 3 4 5] n=5\nE [m n] n=2\n"
	if out != want || st != 0 {
		t.Errorf("set -A = %q (status %d), want %q", out, st, want)
	}
}

// The option parse carries on past the name here, which is the axis. So a
// dash word behind the name is an *option* and a `--` still ends them, and
// the values are exactly the words that would have become the positional
// parameters.
//
//	set -A ff -x -y     -y: unknown option
//	set -A dd -- 1 2    [1 2]
func TestTheOptionsCarryOnPastTheNameHere(t *testing.T) {
	if got := ksh.Semantics().SetArrayOptionsContinuePastTheName; got != interp.Yes {
		t.Errorf("SetArrayOptionsContinuePastTheName = %v, want Yes", got)
	}
	out, st := runKsh(t, t.TempDir(), `set -A dd -- 1 2; echo "A [${dd[@]}] n=${#dd[@]}"`)
	if out != "A [1 2] n=2\n" || st != 0 {
		t.Errorf("set -A dd -- 1 2 = %q (status %d), want two elements and no `--` "+
			"among them", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `set -A ff -x -y`)
	if !strings.Contains(out, "set: -y: unknown option") || st != 2 {
		t.Errorf("set -A ff -x -y = %q (status %d), want the second dash word read "+
			"as an option and refused", out, st)
	}
}

// `set -A a` with no values *unsets* the name here, where zsh leaves an array
// with no elements. Both count 0, so `typeset -p` and `${a+x}` are the only
// places it shows — which is what makes it a field rather than a rule.
func TestSetArrayWithNoValuesUnsetsHere(t *testing.T) {
	if got := ksh.Semantics().SetArrayWithNoValuesUnsetsTheName; got != interp.Yes {
		t.Errorf("SetArrayWithNoValuesUnsetsTheName = %v, want Yes", got)
	}
	out, st := runKsh(t, t.TempDir(), `set -A a 1 2 3
set -A a
echo "A n=${#a[@]} set=[${a+x}] p=[$(typeset -p a 2>&1)]"`)
	if out != "A n=0 set=[] p=[]\n" || st != 0 {
		t.Errorf("set -A with no values = %q (status %d), want the name gone", out, st)
	}
	// The plus form with nothing to put at the front is a different
	// operation and leaves the array alone — unanimous.
	out, st = runKsh(t, t.TempDir(), `set -A c 1 2 3; set +A c; echo "[${c[@]}]"`)
	if out != "[1 2 3]\n" || st != 0 {
		t.Errorf("set +A with no values = %q (status %d), want the array untouched",
			out, st)
	}
}

// `set -A` with no name at all says which operand is missing, and prints
// set's own usage block under it.
func TestSetArrayWithNoNameSaysSoHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `set -A`)
	for _, want := range []string{
		"set: -A: name argument expected",
		"Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("set -A = %q, want %q in it", out, want)
		}
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// A bad name and a frozen name, both of which name the builtin — and the
// frozen one keeps the *builtin* location under it where a plain assignment
// to the same name does not.
//
//	set -A 1bad v     <script>[1]: set: 1bad: invalid variable name
//	set -A ro q       <script>[N]: set: ro: is read only
//	ro=(x y)          <script>: line N: ro: is read only
func TestSetArrayRefusalsNameTheBuiltinHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `set -A 1bad v`)
	if !strings.Contains(out, "set: 1bad: invalid variable name") || st != 1 {
		t.Errorf("set -A 1bad = %q (status %d), want the name refused", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `typeset -r ro=1
set -A ro q`)
	if !strings.Contains(out, "set: ro: is read only") || st != 1 {
		t.Errorf("set -A over a frozen name = %q (status %d), want the builtin "+
			"named", out, st)
	}
	// The control: a plain assignment to the same name does *not* name the
	// builtin, so the naming belongs to this refusal and not to the shell.
	out, _ = runKsh(t, t.TempDir(), `typeset -r ro=1
ro=(x y)`)
	if strings.Contains(out, "set: ") || !strings.Contains(out, "ro: is read only") {
		t.Errorf("ro=(x y) = %q, want no builtin named", out)
	}
}

// `-A` has left the refused-letter list, and it must not be on both: an entry
// there could never be reached now that the letter is claimed, and would say
// the opposite of what the shell does.
func TestTheArrayLetterIsNoLongerCalledMissingHere(t *testing.T) {
	if got := ksh.Diagnostics().UnimplementedOptionLetters["set"]; strings.ContainsRune(got, 'A') {
		t.Errorf("UnimplementedOptionLetters[set] = %q, which still claims -A is "+
			"missing", got)
	}
	out, _ := runKsh(t, t.TempDir(), `set -A a x`)
	if strings.Contains(out, "not implemented yet") || strings.Contains(out, "invalid option") {
		t.Errorf("set -A a x = %q, want it taken", out)
	}
}
