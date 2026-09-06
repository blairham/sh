// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `$0` moves in this dialect and in no other: the FUNCTION_ARGZERO option
// makes it the function being run or the file being sourced, and gives the
// script's name back when that call returns. The corpus rows are
// `axis/dollar-zero-*` in docs/spec/measurements.md, and the reason it
// matters here is a plugin manager that computes its own install directory
// from `${0:h}` inside the file it was sourced from (#978).

// TestDollarZeroFollowsTheSourcedFile walks the routes as behavior, since the
// axis being set is not the same claim as the shell answering with it.
func TestDollarZeroFollowsTheSourcedFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "inc.sh", "echo \"in=[$0]\"\n")
	writeFile(t, dir, "def.sh", "f() { echo \"fn=[$0]\"; }\n")
	writeFile(t, dir, "deep.sh", "echo \"deep=[$0]\"\n")
	writeFile(t, dir, "mid.sh", "echo \"mid=[$0]\"\n. ./deep.sh\necho \"mid-again=[$0]\"\n")

	for _, tc := range []struct{ name, src, want string }{
		{
			"a sourced file, and the name given back after it",
			"echo \"before=[$0]\"\n. ./inc.sh\necho \"after=[$0]\"\n",
			"before=[zsh]\nin=[./inc.sh]\nafter=[zsh]\n",
		},
		{
			// `source` is this dialect's own name for the builtin, and it
			// has to move `$0` the same way.
			"source, which only this dialect and bash have",
			"source ./inc.sh\n",
			"in=[./inc.sh]\n",
		},
		{
			"a function defined in a sourced file, called after it",
			". ./def.sh\ntrue\nf\n",
			"fn=[f]\n",
		},
		{
			"a file sourced by a sourced file",
			". ./mid.sh\n",
			"mid=[./mid.sh]\ndeep=[./deep.sh]\nmid-again=[./mid.sh]\n",
		},
		{
			"a file sourced from inside a function",
			"g() { echo \"g=[$0]\"; . ./inc.sh; echo \"g-again=[$0]\"; }\ng\n",
			"g=[g]\nin=[./inc.sh]\ng-again=[g]\n",
		},
		{
			// Found by the PATH search, which runZsh points at dir: the
			// operand is the answer and not the path that was opened.
			"a file found on PATH",
			". inc.sh\n",
			"in=[inc.sh]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("said %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// TestTheAxisIsSet is the field, which is what the preset promises and what a
// caller building its own Semantics reads.
func TestTheAxisIsSet(t *testing.T) {
	if got := zsh.Semantics().DollarZeroNamesTheInnermostCall; got != interp.Yes {
		t.Errorf("DollarZeroNamesTheInnermostCall = %v, want Yes", got)
	}
}

// TestZshArgzeroIsWhatDollarZeroWasBeforeAnythingMovedIt. The parameter only
// has a meaning because `$0` moves: it is how a sourced file can still find
// out what the shell itself was called, and it is what the
// `${${0:#$ZSH_ARGZERO}:-…}` idiom in that plugin manager tests against.
//
// The prelude is what sets it, so this needs the prelude installed the way
// the front end installs it rather than pasted on the front of the snippet.
func TestZshArgzeroIsWhatDollarZeroWasBeforeAnythingMovedIt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "inc.sh", "echo \"in=[$0] az=[$ZSH_ARGZERO]\"\n")

	out, st := runZshPrelude(t, dir, "echo \"top=[$0] az=[$ZSH_ARGZERO]\"\n. ./inc.sh\n")
	const want = "top=[zsh] az=[zsh]\nin=[./inc.sh] az=[zsh]\n"
	if out != want || st != 0 {
		t.Errorf("said %q status %d, want %q and 0", out, st, want)
	}

	// An ordinary scalar and not a special parameter: real zsh reports
	// `typeset ZSH_ARGZERO=…`, does not export it, and lets a script assign
	// to it or unset it like any other name.
	out, st = runZshPrelude(t, dir, "ZSH_ARGZERO=chosen\necho \"[$ZSH_ARGZERO]\"\n"+
		"unset ZSH_ARGZERO\necho \"[${ZSH_ARGZERO-gone}]\"\n")
	if out != "[chosen]\n[gone]\n" || st != 0 {
		t.Errorf("said %q status %d, want it writable and unsettable", out, st)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
