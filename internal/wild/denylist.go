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

// shellNames are the bare directory names a shell's own files sit under. On
// their own they decide nothing: see shellDistribution.
var shellNames = map[string]bool{
	"bash": true, "dash": true, "zsh": true, "ksh": true, "ksh93": true,
	"mksh": true, "pdksh": true, "oksh": true, "yash": true, "busybox": true,
}

// packageRoots are the directories a package manager keeps one directory per
// formula in, so the segment below one is a package's name.
var packageRoots = map[string]bool{"Cellar": true, "opt": true}

// shellDistribution reports whether the segment at i is a shell's *installed*
// distribution rather than merely a directory the shell looks in.
//
// The distinction is the whole of it, and getting it wrong costs in both
// directions. `/opt/homebrew/Cellar/zsh/5.9.2/share/zsh/5.9/functions` is
// zsh's own code, written by the zsh project, and CLEANROOM.md's red list
// covers it as squarely as the C source does. `/opt/homebrew/share/zsh/
// site-functions` is a directory with the same name holding git's completion,
// brew's and docker's — third-party programs written in zsh, which are the
// most interesting real zsh on a machine and are nobody's shell source.
//
// The tell is what sits next to the name. A distribution is a package root's
// child (`Cellar/zsh`, `opt/bash`) or is versioned (`zsh/5.9`, which is how a
// shell installs its own functions); a place the shell merely looks is neither.
func shellDistribution(segs []string, i int) bool {
	if !shellNames[segs[i]] {
		return false
	}
	if i > 0 && packageRoots[segs[i-1]] {
		return true
	}
	return i+1 < len(segs) && looksLikeVersion(segs[i+1])
}

// looksLikeVersion reports whether a segment is a version number, which is
// what a shell puts between its name and its own files.
func looksLikeVersion(seg string) bool {
	return seg != "" && seg[0] >= '0' && seg[0] <= '9'
}

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
	return deniedSegments(filepath.Dir(path))
}

// DeniedDir reports why CLEANROOM.md forbids descending into the directory at
// path, or "" when the walk may enter it.
//
// It is the same rule as Denied applied one segment further along, and it is
// separate because the two questions are asked about different things. A file
// is denied by what it is *inside*; a directory named zsh is denied by being
// itself, and nothing inside it needs to be looked at to know that.
func DeniedDir(path, own string) string {
	if own != "" && underDir(path, own) {
		return ""
	}
	return deniedSegments(path)
}

// deniedSegments scans a directory path for a segment that names someone
// else's tree.
func deniedSegments(dir string) string {
	segs := strings.Split(filepath.ToSlash(dir), "/")
	for i, seg := range segs {
		for _, marker := range shellTreeMarkers {
			if strings.HasPrefix(seg, marker) {
				return ReasonShellSource
			}
		}
		if shellDistribution(segs, i) {
			return ReasonShellSource
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
