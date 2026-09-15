// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `$0` and the `function` keyword.
//
// This shell is the panel's third answer to what `$0` names inside a call:
// not the shell's own name however deep it is (bash, dash, ash) and not the
// innermost call whatever it is (zsh), but the nearest function that was
// *defined with the `function` keyword*. Nothing else moves it — a `name()`
// function does not, a sourced file does not — and neither of them hides a
// keyword function further down the stack.
//
// Measured 2026-09-15 on ksh93u+ 2012-08-01, from a script file:
//
//	function kf { echo "$0"; . ./inc.sh; echo "$0"; }   kf, kf, kf
//	pf() { echo "$0"; }                                 the script's path
//	. ./inc.sh at the top level                         the script's path
//	function outer { pf; }                              outer
//
// The third and fourth rows are the ones that say which frame is being asked.
// A sourced file is the top of the stack in the shell that names the
// innermost call and reports itself there; here it reports the function that
// sourced it. And `pf` called from inside `outer` reports `outer`, so the
// frames that do not answer are transparent rather than an answer of their
// own.
func TestDollarZeroNamesTheInnermostKeywordFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.sh"), []byte("echo \"in=[$0]\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"the keyword spelling names the function",
			"function kf { echo \"kf=[$0]\"; }\nkf\n",
			"kf=[kf]\n",
		},
		{
			"and the parenthesis spelling does not",
			"pf() { echo \"pf=[$0]\"; }\npf\n",
			"pf=[ksh]\n",
		},
		{
			"a sourced file does not answer and does not hide the function",
			"function kf { . ./inc.sh; }\nkf\n",
			"in=[kf]\n",
		},
		{
			"nor at the top level",
			". ./inc.sh\n",
			"in=[ksh]\n",
		},
		{
			"a name() function inside a keyword one is still the outer",
			"pf() { echo \"pf=[$0]\"; }\nfunction outer { pf; }\nouter\n",
			"pf=[outer]\n",
		},
		{
			"and it goes back when the call returns",
			"function kf { :; }\nkf\necho \"after=[$0]\"\n",
			"after=[ksh]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// TestTheDollarZeroAxisIsSet is the field, which is what the preset promises
// and what a caller building its own Semantics reads.
func TestTheDollarZeroAxisIsSet(t *testing.T) {
	if got := ksh.Semantics().DollarZeroNames; got != interp.DollarZeroIsTheInnermostKeywordFunction {
		t.Errorf("DollarZeroNames = %v, want the innermost keyword function", got)
	}
}
