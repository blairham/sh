// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// renamedCdRow runs a snippet in a directory that renames itself out from
// under the shell, and answers what the shell said.
//
// The rename is `mv`, run by the shell as an external command, because that is
// what the case is: the directory goes while the shell is in it and nothing in
// the shell is told. PATH is the machine's own so the command is found.
func renamedCdRow(t *testing.T, src string) (string, int) {
	t.Helper()
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if err := os.MkdirAll(filepath.Join(root, "d", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir:  filepath.Join(root, "d"),
		Vars: map[string]string{"PATH": "/usr/bin:/bin"},
	}, "cd .\nmv ../d ../e\n"+src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `cd .` in a directory that has been renamed moves, and `$PWD` becomes the
// new name. Measured 2026-09-26 on zsh 5.9.2 (`-f`), `go version -m` says *not
// a Go executable* for it: with `d` renamed to `e`, `cd .` is 0 and `$PWD`
// reads `…/e`. It is the last chunk of `B01cd.ztst` (#4653) and closing it
// closes the file.
func TestCdFromARenamedDirectory(t *testing.T) {
	if got := zsh.Semantics().CdDestinationIsNotThere; got != interp.CdDestinationNotThereEntersAndTakesTheKernelsName {
		t.Fatalf("the preset answers %v, want interp.CdDestinationNotThereEntersAndTakesTheKernelsName", got)
	}
	out, _ := renamedCdRow(t, `cd .`+"\n"+`echo "st=$? at=${PWD##*/}"`+"\n")
	if out != "st=0 at=e\n" {
		t.Errorf("`cd .` in a renamed directory said %q, want %q", out, "st=0 at=e\n")
	}
}

// And the half that is not on the axis: whatever `cd` decides, the shell is
// still *in* the directory, so an external command it starts runs there and a
// relative redirection lands there. Unanimous across all six real shells,
// measured the same day, which is why it is asserted in every dialect and
// answered by none.
func TestARelativePathStillReachesARenamedDirectory(t *testing.T) {
	out, _ := renamedCdRow(t, `echo hi > rel.txt`+"\n"+`ls ../e`+"\n")
	if out != "rel.txt\ns\n" {
		t.Errorf("after the rename the shell wrote %q, want rel.txt beside s in the renamed directory", out)
	}
}

// `pwd -P` from a renamed directory names where the directory *is* — #4667.
//
// Measured 2026-09-26 on zsh 5.9.2 (`-f`): with `d` renamed to `e`, `pwd`
// still writes `…/d` and `pwd -P` writes `…/e`. bash 5.3, bash 3.2, dash and
// BusyBox ash agree; ksh93 alone writes `…/d`, which is the same column that
// keeps the built name after a `cd`. The plain `pwd` row is the control and
// it is what says this is about `-P` rather than about the shell losing its
// name.
func TestPwdPhysicalFollowsARenamedDirectory(t *testing.T) {
	out, _ := renamedCdRow(t, `echo "pwd=${$(pwd)##*/} pwdP=${$(pwd -P)##*/}"`+"\n")
	if out != "pwd=d pwdP=e\n" {
		t.Errorf("after the rename the shell said %q, want %q", out, "pwd=d pwdP=e\n")
	}
}

// `cd ..` out of a directory whose parent has been renamed **and** had its
// name taken by another directory follows the directory the shell is in, not
// the name that came back — #4668.
//
// Measured 2026-09-26 on zsh 5.9.2 (`-f`), with the shell in `…/d/s`, `d`
// renamed to `e`, and a new `d` holding a file of its own: `$PWD` reads `…/e`
// and `*` lists `s`. ksh93, dash and BusyBox ash all land on the impostor
// instead, which is why the axis has three readings and not two. See
// interp.Semantics.CdCancelsADotDot.
func TestCdDotDotFollowsTheDirectoryAndNotTheName(t *testing.T) {
	if got := zsh.Semantics().CdCancelsADotDot; got != interp.CdDotDotLooksAtEveryCanceledComponent {
		t.Fatalf("the preset answers %v, want interp.CdDotDotLooksAtEveryCanceledComponent", got)
	}
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if err := os.MkdirAll(filepath.Join(root, "d", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir:  filepath.Join(root, "d", "s"),
		Vars: map[string]string{"PATH": "/usr/bin:/bin"},
	}, "cd .\nmv ../../d ../../e\nmkdir ../../d\ntouch ../../d/IMPOSTOR\n"+
		`cd ..`+"\n"+`echo "at=${PWD##*/} ls=$(echo *)"`+"\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "at=e ls=s\n" || st != 0 {
		t.Errorf("`cd ..` said %q (status %d), want %q", out, st, "at=e ls=s\n")
	}
}

// `cd nosuch/..` is refused: the component the `..` cancels is looked at
// first. `cd real/..` in the same tree is 0, which is the control — the
// refusal is about the component and not about the `..`.
func TestCdLooksAtTheComponentADotDotCancels(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, st := runZsh(t, dir, "cd nosuch/.."); st != 1 {
		t.Errorf("`cd nosuch/..` was %d, want 1", st)
	}
	if _, st := runZsh(t, dir, "cd real/.."); st != 0 {
		t.Errorf("`cd real/..` was %d, want 0", st)
	}
}
