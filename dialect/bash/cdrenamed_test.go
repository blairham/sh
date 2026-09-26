// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
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

// The same answer as zsh, and measured in both builds: /opt/homebrew/bin/bash
// (5.3) and /bin/bash (3.2) on 2026-09-26 each answer `cd .` in a renamed
// directory with 0 and `$PWD` reading `…/e`. Two builds twenty years apart
// agreeing is what says this is bash's answer rather than a version's.
func TestCdFromARenamedDirectory(t *testing.T) {
	if got := bash.Semantics().CdDestinationIsNotThere; got != interp.CdDestinationNotThereEntersAndTakesTheKernelsName {
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

// `cd nosuch/..` is refused here — the component a `..` cancels is looked
// at first, whether it came from the operand or from the directory the
// shell is in. See
// interp.Semantics.CdCancelsADotDot for the grid the panel splits on (#4627).
//
// `cd real/..` in the same tree is 0 in all six columns and is the control:
// what the row above says is that the component is looked at, not that a `..`
// is refused.
func TestCdAndTheComponentADotDotCancels(t *testing.T) {
	if got := bash.Semantics().CdCancelsADotDot; got != interp.CdDotDotLooksAtEveryCanceledComponent {
		t.Fatalf("the preset answers %v, want interp.CdDotDotLooksAtEveryCanceledComponent", got)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, st := runBash(t, dir, "cd nosuch/.."); st != 1 {
		t.Errorf("`cd nosuch/..` was %d, want 1", st)
	}
	if _, st := runBash(t, dir, "cd real/.."); st != 0 {
		t.Errorf("`cd real/..` was %d, want 0", st)
	}
}
