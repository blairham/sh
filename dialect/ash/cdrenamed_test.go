// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
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

// BusyBox ash refuses, as dash does — measured rather than derived from it.
// 2026-09-26, in the pinned alpine digest, BusyBox v1.37.0: `cd .` in a
// renamed directory writes `cd: line 0: can't cd to .: No such file or
// directory` and exits 2, while `/bin/pwd` in the same shell prints the new
// name and a relative `touch` lands in it.
func TestCdFromARenamedDirectory(t *testing.T) {
	if got := ash.Semantics().CdDestinationIsNotThere; got != interp.CdDestinationNotThereRefuses {
		t.Fatalf("the preset answers %v, want interp.CdDestinationNotThereRefuses", got)
	}
	out, _ := renamedCdRow(t, `cd .`+"\n"+`echo "st=$? at=${PWD##*/}"`+"\n")
	if !strings.HasSuffix(out, "st=2 at=d\n") {
		t.Errorf("`cd .` in a renamed directory said %q, want it to end st=2 at=d", out)
	}
	// The sentence too, because a refusal that says the wrong thing is a
	// different shell: this is the reference's own wording for the case, and
	// it is the operating system's reason rather than one made up here.
	if !strings.Contains(out, `cd: line 3: can't cd to .: No such file or directory`) {
		t.Errorf("the refusal said %q, want %q in it", out, `cd: line 3: can't cd to .: No such file or directory`)
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
