// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
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

// ksh93 moves and keeps the name it had already built. Measured 2026-09-26 on
// /bin/ksh (AT&T 93u+): `cd .` in a renamed directory is 0 and `$PWD` stays
// `…/d`. The move is real — `ls` in there lists the renamed directory's own
// entries — and only the name is stale, which is the same column never asking
// the kernel where it is.
func TestCdFromARenamedDirectory(t *testing.T) {
	if got := ksh.Semantics().CdDestinationIsNotThere; got != interp.CdDestinationNotThereEntersAndKeepsTheBuiltName {
		t.Fatalf("the preset answers %v, want interp.CdDestinationNotThereEntersAndKeepsTheBuiltName", got)
	}
	out, _ := renamedCdRow(t, `cd .`+"\n"+`echo "st=$? at=${PWD##*/}"`+"\n")
	if out != "st=0 at=d\n" {
		t.Errorf("`cd .` in a renamed directory said %q, want %q", out, "st=0 at=d\n")
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
// Measured 2026-09-26 with `d` renamed to `e` while the shell sat in it: `pwd`
// still writes `…/d` and `pwd -P` writes `…/e` in zsh 5.9.2, bash 5.3, bash
// 3.2, dash and BusyBox ash 1.37, while ksh93 alone writes `…/d`. That is the
// same column that keeps the built name after a `cd`, so no second axis is
// needed — see interp.Semantics.CdDestinationIsNotThere. The plain `pwd` row
// is the control and it is what says this is about `-P` rather than about the
// shell losing its name.
func TestPwdPhysicalFollowsARenamedDirectory(t *testing.T) {
	out, _ := renamedCdRow(t, `p=$(pwd); q=$(pwd -P); echo "pwd=${p##*/} pwdP=${q##*/}"`+"\n")
	want := "pwd=d pwdP=d\n"
	if !strings.HasSuffix(out, want) {
		t.Errorf("after the rename the shell said %q, want it to end %q", out, want)
	}
}

// `cd nosuch/..` is refused here — the component a `..` cancels is looked
// at where it belongs to the operand, and canceled unseen where it belongs
// to `$PWD` — which is neither of the answers the other four hold. See
// interp.Semantics.CdCancelsADotDot for the grid the panel splits on (#4627).
//
// `cd real/..` in the same tree is 0 in all six columns and is the control:
// what the row above says is that the component is looked at, not that a `..`
// is refused.
func TestCdAndTheComponentADotDotCancels(t *testing.T) {
	if got := ksh.Semantics().CdCancelsADotDot; got != interp.CdDotDotLooksWithinTheOperand {
		t.Fatalf("the preset answers %v, want interp.CdDotDotLooksWithinTheOperand", got)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, st := runKsh(t, dir, "cd nosuch/.."); st != 1 {
		t.Errorf("`cd nosuch/..` was %d, want 1", st)
	}
	if _, st := runKsh(t, dir, "cd real/.."); st != 0 {
		t.Errorf("`cd real/..` was %d, want 0", st)
	}
}

// And the row the operand table above cannot see: a `..` that reaches past the
// operand into the directory the shell is already in is canceled **unseen**
// here, so `cd ..` out of a directory whose parent has been renamed and whose
// old name has been taken by another directory lands on the impostor.
//
// Measured 2026-09-26 with the shell in `…/d/s`, `d` renamed to `e` and a new
// `d` made at the old name with a file of its own: `$PWD` reads `…/d` and `*`
// lists that file. dash and BusyBox ash answer the same; bash 5.3 and zsh
// 5.9.2 answer `…/e` and list `s`, which is what makes this column a third
// reading rather than a rounding of either.
//
// `cd ./..` is the same row with a `.` in front of it, so what decides is the
// component the `..` reaches rather than what the operand starts with.
//
// Read on both builds: `/bin/ksh`, AT&T's 93u+m of 2012 on this machine, and
// `ksh93u+m 1.0.10-7` from Debian sid in a container, which is the maintained
// lineage. They agree on every row, so this is the shell and not the fork.
// See interp.Semantics.CdCancelsADotDot (#4627, #4668).
func TestADotDotIntoThePwdIsCanceledUnseen(t *testing.T) {
	for _, src := range []string{"cd ..", "cd ./.."} {
		// A tree per row, because each of them renames its own away.
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
			src+"\n"+`echo "at=${PWD##*/} ls=$(echo *)"`+"\n")
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		if out != "at=d ls=IMPOSTOR\n" || st != 0 {
			t.Errorf("`%s` said %q (status %d), want %q", src, out, st, "at=d ls=IMPOSTOR\n")
		}
	}
}
