// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `cd -s` refuses an operand that crosses a symbolic link, and this shell is
// the only one of the panel that has the letter at all — bash 5.3.15, that
// binary under argv[0] `sh`, bash 3.2.57, dash and ksh93 all refuse `-s` by
// name, exactly as they refuse `-q`. Measured 2026-09-12 on zsh 5.9.2 (#1569).
//
// Without the letter the word was an operand, so `cd -s dir` went looking for
// a directory called `-s` — the same failure `-q` had before #1558, and the
// reason a letter this shell has is never a free one here.
func TestCdSymlinkFreeIsThisShellsLetter(t *testing.T) {
	if got := zsh.Semantics().CdHasSymlinkFreeOption; got != interp.Yes {
		t.Fatalf("this shell has `cd -s`; the preset says %v", got)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ src, want string }{
		{`cd -s real; echo "st=$? at=${PWD##*/}"`, "st=0 at=real"},
		{`cd -s link; echo "st=$?"`, "st=1"},
		{`cd -s link/deep; echo "st=$?"`, "st=1"},
		{`cd -s real/../real; echo "st=$? at=${PWD##*/}"`, "st=0 at=real"},
		// The walk is over the operand and not over the place arrived at:
		// having moved into the link, a further `cd -s deep` moves.
		{`cd link; cd -s deep; echo "st=$? at=${PWD##*/}"`, "st=0 at=deep"},
		// A component that is not there is the ordinary failure, not this
		// one — the walk stops before it has anything to refuse.
		{`cd -s real/nosuch; echo "st=$?"`, "st=1"},
	} {
		out, _ := runZsh(t, dir, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: out %q, want %q in it", c.src, out, c.want)
		}
	}
}

// The two refusals are different sentences, which is the half a status check
// cannot see: a link is `not a directory` and a path that is not there is
// `no such file or directory`. Saying either for both is an answer this
// shell can be shown to disagree with.
func TestCdSymlinkFreeRefusalIsNotTheMissingOne(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	linked, _ := runZsh(t, dir, `cd -s link`)
	if !strings.Contains(linked, "not a directory: link") {
		t.Errorf("`cd -s link` said %q, want `not a directory: link`", linked)
	}
	missing, _ := runZsh(t, dir, `cd -s nosuch`)
	if !strings.Contains(missing, "no such file or directory: nosuch") {
		t.Errorf("`cd -s nosuch` said %q, want the missing-directory reason", missing)
	}
}
