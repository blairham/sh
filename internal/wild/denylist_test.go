// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/wild"
)

// The deny-list classifies from the path alone, because opening the file is
// exactly what CLEANROOM.md forbids. These are the paths the sweep must
// refuse and the paths it must not.
func TestDeniedClassifiesFromThePathAlone(t *testing.T) {
	for _, tc := range []struct {
		path, want string
	}{
		// Another shell's own distribution — the issue's real sightings.
		{"/src/bash-3.2/examples/scripts/adventure.sh", wild.ReasonShellSource},
		{"/src/bash-3.2/examples/scripts.v2/ren", wild.ReasonShellSource},
		{"/home/x/dash-0.5.12/src/mksignames.sh", wild.ReasonShellSource},
		{"/tmp/zsh-5.9/Functions/Misc/zargs", wild.ReasonShellSource},
		{"/opt/ksh-93u+/src/cmd/ksh93/foo.sh", wild.ReasonShellSource},
		{"/build/busybox-1.36.1/shell/ash_test/run", wild.ReasonShellSource},
		{"/go/pkg/mod/mvdan.cc/sh/v3@v3.8.0/syntax/parser.sh", wild.ReasonShellSource},
		// The marker wins wherever it sits in the path, and a shell tree's
		// own tests directory is still the shell tree.
		{"/src/bash-3.2/tests/run-all", wild.ReasonShellSource},

		// Another project's test data — vim's is the issue's real sighting.
		{"/usr/share/vim/vim91/syntax/testdir/input/sh_12.sh", wild.ReasonTestData},
		{"/usr/share/vim/vim91/syntax/testdir/input/sh_bash.bash", wild.ReasonTestData},
		{"/usr/lib/node_modules/pkg/testdata/hook.sh", wild.ReasonTestData},
		{"/opt/someproject/tests/regress.sh", wild.ReasonTestData},

		// What the sweep actually points at must keep flowing.
		{"/usr/bin/ldd", ""},
		{"/opt/homebrew/bin/brew", ""},
		{"/usr/local/sbin/backup.sh", ""},
		// A *file* whose name carries a marker is not inside anyone's tree —
		// only directories on the way to the file classify it.
		{"/usr/local/bin/bash-wrapper", ""},
		{"/usr/local/bin/tests", ""},
		// Markers match directory-name prefixes, not substrings.
		{"/usr/lib/rebash-2.0/lib.sh", ""},
		{"/opt/latest/run.sh", ""},
	} {
		if got := wild.Denied(tc.path, ""); got != tc.want {
			t.Errorf("Denied(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// This repository's own test files are ours to read — CLEANROOM.md bans
// *other* projects' test data — so the tree the sweep runs from is exempt.
func TestDeniedExemptsOurOwnTree(t *testing.T) {
	own := "/home/me/sh"
	for _, tc := range []struct {
		path, want string
	}{
		{"/home/me/sh/internal/oracle/testdata/record.sh", ""},
		{"/home/me/sh/tests/case.sh", ""},
		// A sibling whose name merely extends ours is not under our tree.
		{"/home/me/sh-other/testdata/case.sh", wild.ReasonTestData},
		{"/home/me/other/testdata/case.sh", wild.ReasonTestData},
	} {
		if got := wild.Denied(tc.path, own); got != tc.want {
			t.Errorf("Denied(%q, %q) = %q, want %q", tc.path, own, got, tc.want)
		}
	}
}

// A denied file is counted and never surfaced: the sweep must not open it
// even to read the shebang, and the report must not hand anyone its path.
func TestFindCountsDeniedFilesWithoutOpeningThem(t *testing.T) {
	dir := t.TempDir()
	shellTree := filepath.Join(dir, "bash-3.2", "examples")
	testTree := filepath.Join(dir, "vim", "testdir")
	for _, d := range []string{shellTree, testTree} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Unreadable on purpose: if Find tries to open them, the count breaks.
	write(t, shellTree, "adventure.sh", "#!/bin/sh\necho hi\n")
	write(t, testTree, "input.sh", "#!/bin/sh\necho hi\n")
	write(t, testTree, "not-even-a-script", "whether this is shell is unknowable without reading it\n")
	for _, p := range []string{
		filepath.Join(shellTree, "adventure.sh"),
		filepath.Join(testTree, "input.sh"),
		filepath.Join(testTree, "not-even-a-script"),
	} {
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
	}
	allowed := write(t, dir, "fine.sh", "#!/bin/sh\necho hi\n")

	paths, skipped := wild.Find([]string{dir, shellTree, testTree})
	if len(paths) != 1 || paths[0] != allowed {
		t.Errorf("paths = %v, want just %s", paths, allowed)
	}
	if skipped[wild.ReasonShellSource] != 1 {
		t.Errorf("skipped[%q] = %d, want 1", wild.ReasonShellSource, skipped[wild.ReasonShellSource])
	}
	if skipped[wild.ReasonTestData] != 2 {
		t.Errorf("skipped[%q] = %d, want 2", wild.ReasonTestData, skipped[wild.ReasonTestData])
	}
}
