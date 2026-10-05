// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestADotDotReachesTheKernelAsWritten is #6081: every route that hands a
// path the script wrote to the operating system used to clean it first, so a
// `..` was canceled by text against the component before it. The kernel
// resolves a `..` physically — the component has to be a directory that is
// there, and the parent of a link is the parent of where it leads — and so
// does every shell in the panel. Measured 2026-10-05 against bash 5.3, zsh
// 5.9.2, ksh93u+ and dash, all four agreeing on every line below.
//
// One line per route, each asked so that the lexical reading and the physical
// one give different answers:
//
//   - `sub/nosuch/../../f` names nothing, since `sub/nosuch` is not a
//     directory to take the parent of; cleaned, it is `f`, which is there.
//   - `sub/fake/..` is the root, `fake` pointing at `../real`; cleaned, it is
//     `sub`. So `g`, which is only in the root, is found through it and
//     `fake`, which is only in `sub`, is not.
//   - `f/` is a file asked to be a directory; cleaned, the slash is gone.
//
// Every one of these lines came out the other way before the fix.
func TestADotDotReachesTheKernelAsWritten(t *testing.T) {
	root := dotDotTree(t)
	write := func(name, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("f", "echo sourced\n", 0o644)
	write("g", "", 0o644)
	write("exe", "#!/bin/sh\necho ran\n", 0o755)
	write("tool", "#!/bin/sh\necho tool\n", 0o755)

	src := `[ -e sub/nosuch/../../f ]; echo "test=$?"
[ -e sub/fake/../g ]; echo "through=$?"
[ -e sub/fake/../fake ]; echo "back=$?"
[ -e f/ ]; echo "slash=$?"
{ read x < sub/nosuch/../../f; } 2>/dev/null; echo "redir=$?"
(. sub/nosuch/../../f) 2>/dev/null; echo "dot=$?"
(. sub/fake/../f); echo "dotlink=$?"
sub/nosuch/../../exe 2>/dev/null; echo "exec=$?"
PATH=` + root + `/sub/fake/..; tool; echo "path=$?"
`
	out, _ := run(t, src, func(r *Runner) { r.Dir = root })
	want := `test=1
through=0
back=1
slash=1
redir=1
dot=2
sourced
dotlink=0
exec=127
tool
path=0
`
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}
