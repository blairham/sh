// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `set -A name value …`, which this shell has and bash and dash do not.
// Measured 2026-09-06 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, ZDOTDIR and HISTFILE, over a script file.
//
// It is on the release bar because this shell's own `add-zsh-hook` is written
// with it — `set -A $hook ${(P)hook} $fn` — which is the array assignment
// through a name a variable holds, and the thing `name=(…)` cannot express.

// The assignment, the base it reads back at, and the plus form.
func TestSetArrayAssignsHere(t *testing.T) {
	if got := zsh.Semantics().SetArrayLetter; got != interp.Yes {
		t.Errorf("SetArrayLetter = %v, want Yes", got)
	}
	out, st := runZsh(t, t.TempDir(), `set -A a x y z; echo "A st=$? [${a[@]}] n=${#a[@]}"
echo "B zero=[${a[0]}] one=[${a[1]}]"
set -A a p; echo "C [${a[@]}] n=${#a[@]}"
set -A b 1 2 3 4 5
set +A b Q R; echo "D [${b[@]}] n=${#b[@]}"
h=hh; set -A $h m n; echo "E [${hh[@]}] n=${#hh[@]}"`)
	// The base is this shell's, counted from one, and it is read through the
	// same whole-array store `name=(…)` reaches rather than answered twice.
	want := "A st=0 [x y z] n=3\nB zero=[] one=[x]\nC [p] n=1\n" +
		"D [Q R 3 4 5] n=5\nE [m n] n=2\n"
	if out != want || st != 0 {
		t.Errorf("set -A = %q (status %d), want %q", out, st, want)
	}
}

// The name ends the options here, which is the axis: every word behind it is
// a value, dash words and `--` included. ksh93 keeps parsing and answers both
// of these the other way.
func TestTheNameEndsTheOptionsHere(t *testing.T) {
	if got := zsh.Semantics().SetArrayOptionsContinuePastTheName; got != interp.No {
		t.Errorf("SetArrayOptionsContinuePastTheName = %v, want No", got)
	}
	out, st := runZsh(t, t.TempDir(), `set -A ff -x -y; echo "A [${ff[@]}] n=${#ff[@]}"
set -A dd -- 1 2; echo "B [${dd[@]}] n=${#dd[@]}"`)
	want := "A [-x -y] n=2\nB [-- 1 2] n=3\n"
	if out != want || st != 0 {
		t.Errorf("dash words after the name = %q (status %d), want %q", out, st, want)
	}
}

// `set -A a` with no values leaves an array with no elements here, where
// ksh93 unsets the name. Both count 0, so `typeset -p` and `${a+x}` are the
// only places it shows.
func TestSetArrayWithNoValuesLeavesAnEmptyArrayHere(t *testing.T) {
	if got := zsh.Semantics().SetArrayWithNoValuesUnsetsTheName; got != interp.No {
		t.Errorf("SetArrayWithNoValuesUnsetsTheName = %v, want No", got)
	}
	out, st := runZsh(t, t.TempDir(), `set -A a 1 2 3
set -A a
echo "A n=${#a[@]} set=[${a+x}]"
typeset -p a`)
	want := "A n=0 set=[x]\ntypeset -a a=(  )\n"
	if out != want || st != 0 {
		t.Errorf("set -A with no values = %q (status %d), want %q — the name still "+
			"there and empty", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `set -A c 1 2 3; set +A c; echo "[${c[@]}]"`)
	if out != "[1 2 3]\n" || st != 0 {
		t.Errorf("set +A with no values = %q (status %d), want the array untouched",
			out, st)
	}
}

// `set -A` with no name answers with a listing of every array this shell has,
// which is not built — so the listing is named as missing rather than the
// whole array table being written, and rather than a refusal this shell does
// not make being invented for it.
func TestSetArrayWithNoNameNamesTheListingAsMissingHere(t *testing.T) {
	if got := zsh.Diagnostics().SetArrayNeedsAName; got != "" {
		t.Errorf("SetArrayNeedsAName = %q, want empty — this shell answers a "+
			"missing name with a listing rather than a complaint", got)
	}
	out, _ := runZsh(t, t.TempDir(), `set -A`)
	if !strings.Contains(out, ":set:1: -A: a listing is not implemented yet") {
		t.Errorf("set -A = %q, want the listing named as missing", out)
	}
	if strings.Contains(out, "name argument expected") {
		t.Errorf("set -A = %q, want no complaint this shell does not make", out)
	}
}

// A bad name and a frozen name, and neither names the builtin in its
// location here — where `unset 1x` and `typeset 1w` from this same shell do.
//
//	set -A 1v q       <script>:1: not an identifier: 1v
//	unset 1x          <script>:unset:1: 1x: invalid parameter name
//	set -A ro q       <script>:N: read-only variable: ro
func TestSetArrayRefusalsDoNotNameTheBuiltinHere(t *testing.T) {
	if !zsh.Diagnostics().BadNameRefusalHidesTheBuiltin["set"] {
		t.Error("BadNameRefusalHidesTheBuiltin has no `set` entry")
	}
	out, st := runZsh(t, t.TempDir(), `set -A 1v q`)
	if !strings.Contains(out, ":1: not an identifier: 1v") || st != 1 {
		t.Errorf("set -A 1v q = %q (status %d), want the name refused", out, st)
	}
	if strings.Contains(out, ":set:") {
		t.Errorf("set -A 1v q = %q, want no builtin in the location", out)
	}
	// The control from the same shell, which *does* name it — so the
	// hiding belongs to this builtin and not to the dialect.
	out, _ = runZsh(t, t.TempDir(), `unset 1x`)
	if !strings.Contains(out, ":unset:") {
		t.Errorf("unset 1x = %q, want the builtin named — otherwise the entry "+
			"above is hiding nothing", out)
	}
	out, st = runZsh(t, t.TempDir(), `typeset -r ro=1
set -A ro q`)
	if !strings.Contains(out, "read-only variable: ro") || st != 1 {
		t.Errorf("set -A over a frozen name = %q (status %d), want it refused", out, st)
	}
	if strings.Contains(out, ":set:") {
		t.Errorf("set -A over a frozen name = %q, want no builtin in the location",
			out)
	}
}

// An association is a different operation under the same spelling and the two
// shells do not agree which — key-and-value pairs here, counted elements in
// ksh93 — so it is named as missing rather than one of the two being picked.
//
// The builtin rides in the *location* here rather than in the sentence, which
// is this shell's shape for every builtin's complaint and is why the assertion
// asks for `:set:`.
func TestAnAssociationTargetIsNamedAsMissingHere(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `typeset -A m
set -A m k1 v1`)
	if !strings.Contains(out, ":set:2: -A over an association is not implemented yet") {
		t.Errorf("set -A over an association = %q, want it named as missing", out)
	}
}

// The line this shell's own `add-zsh-hook` installs a hook with, end to end.
func TestTheLineAddZshHookAssignsAHookWith(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `hook=precmd_functions
typeset -ga $hook
fn=my_hook
set -A $hook ${(P)hook} $fn
echo "n=${#precmd_functions[@]} [${precmd_functions[@]}]"`)
	want := "n=1 [my_hook]\n"
	if out != want || st != 0 {
		t.Errorf("add-zsh-hook's assignment = %q (status %d), want %q", out, st, want)
	}
}

// `-A` has left the refused-letter list, and it must not be on both.
func TestTheArrayLetterIsNoLongerCalledMissingHere(t *testing.T) {
	if got := zsh.Diagnostics().UnimplementedOptionLetters["set"]; strings.ContainsRune(got, 'A') {
		t.Errorf("UnimplementedOptionLetters[set] = %q, which still claims -A is "+
			"missing", got)
	}
	out, _ := runZsh(t, t.TempDir(), `set -A a x`)
	if strings.Contains(out, "not implemented yet") || strings.Contains(out, "bad option") {
		t.Errorf("set -A a x = %q, want it taken", out)
	}
}
