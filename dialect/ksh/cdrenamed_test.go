// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
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
