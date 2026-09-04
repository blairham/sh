// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild

import (
	"path/filepath"
	"strings"
)

// The reasons the sweep refuses to open a file, worded the way the report
// prints them: "N in <reason>".
const (
	// ReasonShellSource marks a path inside another shell implementation's
	// own distribution. CLEANROOM.md's red list bans reading every one of
	// them, whatever the license — and bash's is GPLv3, which is worse.
	ReasonShellSource = "another shell's source tree"
	// ReasonTestData marks a path inside another project's test directory.
	// Test files are expression, exactly as protected as the implementation
	// they test.
	ReasonTestData = "another project's test data"
)

// shellTreeMarkers are directory-name prefixes that identify a shell
// implementation's source tree: an unpacked distribution (bash-3.2,
// dash-0.5.12, zsh-5.9, ksh-93u+, busybox-1.36) or a checkout of the Go
// incumbent (mvdan.cc). Matching a prefix over-skips a little — a
// bash-completion tree is not a shell — and that is the right direction to
// err in, because a skipped file is counted rather than lost.
var shellTreeMarkers = []string{"bash-", "dash-", "zsh-", "ksh-", "busybox", "mvdan"}

// foreignTestDirs are the directory names projects keep their test files
// under. vim's syntax tests use testdir; Go projects use testdata; tests is
// everyone else's.
var foreignTestDirs = map[string]bool{"testdir": true, "testdata": true, "tests": true}

// Denied reports why CLEANROOM.md forbids opening the file at path, or ""
// when it may be read.
//
// The classification uses the path alone, never the file's content, because
// reading the content is exactly what is being refused. For the same reason
// a denied path must be counted and never printed: a path in a report is an
// invitation to go look, and looking is the one thing the reader must not
// do.
//
// Only the directories leading to the file are examined. A *file* named
// bash-upgrade or tests is not inside anyone's source tree, and shell
// distributions ship as directories, not loose files.
//
// own, when non-empty, is the root of the working tree the sweep runs from,
// and nothing under it is denied: this project's testdata is generated from
// oracle runs and is ours to read, so the test-directory rule marks only
// someone else's tree as off-limits.
func Denied(path, own string) string {
	if own != "" && underDir(path, own) {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	for _, seg := range strings.Split(dir, "/") {
		for _, marker := range shellTreeMarkers {
			if strings.HasPrefix(seg, marker) {
				return ReasonShellSource
			}
		}
		if foreignTestDirs[seg] {
			return ReasonTestData
		}
	}
	return ""
}

// underDir reports whether path is dir itself or lies inside it.
func underDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
