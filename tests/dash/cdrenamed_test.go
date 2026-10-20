// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
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

// dash refuses. Measured 2026-09-26 on /bin/dash: `cd .` in a renamed
// directory writes `cd: can't cd to .` and exits 2, and `$PWD` does not move.
// Its children still run in the renamed directory all the same, because the
// kernel holds dash's directory for it — so this is a decision about `cd` and
// not a shell that has lost track of where it is.
func TestCdFromARenamedDirectory(t *testing.T) {
	if got := dash.Semantics().CdDestinationIsNotThere; got != interp.CdDestinationNotThereRefuses {
		t.Fatalf("the preset answers %v, want interp.CdDestinationNotThereRefuses", got)
	}
	out, _ := renamedCdRow(t, `cd .`+"\n"+`echo "st=$? at=${PWD##*/}"`+"\n")
	if !strings.HasSuffix(out, "st=2 at=d\n") {
		t.Errorf("`cd .` in a renamed directory said %q, want it to end st=2 at=d", out)
	}
	// The sentence too, because a refusal that says the wrong thing is a
	// different shell: this is the reference's own wording for the case, and
	// it is the operating system's reason rather than one made up here.
	if !strings.Contains(out, `cd: can't cd to .`) {
		t.Errorf("the refusal said %q, want %q in it", out, `cd: can't cd to .`)
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
	want := "pwd=d pwdP=e\n"
	if !strings.HasSuffix(out, want) {
		t.Errorf("after the rename the shell said %q, want it to end %q", out, want)
	}
}

// `cd nosuch/..` is 0 here — the component a `..` cancels is canceled
// against the `..` without either being looked at. See
// interp.Semantics.CdCancelsADotDot for the grid the panel splits on (#4627).
//
// `cd real/..` in the same tree is 0 in all six columns and is the control:
// what the row above says is that the component is looked at, not that a `..`
// is refused.
func TestCdAndTheComponentADotDotCancels(t *testing.T) {
	if got := dash.Semantics().CdCancelsADotDot; got != interp.CdDotDotCanceledUnseen {
		t.Fatalf("the preset answers %v, want interp.CdDotDotCanceledUnseen", got)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, st := runDash(t, dir, "cd nosuch/.."); st != 0 {
		t.Errorf("`cd nosuch/..` was %d, want 0", st)
	}
	if _, st := runDash(t, dir, "cd real/.."); st != 0 {
		t.Errorf("`cd real/..` was %d, want 0", st)
	}
}
