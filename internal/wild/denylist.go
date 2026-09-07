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

// foreignTestDirs are the directory names a project keeps its test files
// under, matched against the whole name and against the name's first word.
//
// The list was three entries — testdir, testdata, tests — and singular `test`
// was not one of them, so `<pkg>/share/ncurses/test` was swept like any other
// directory and nine of another project's test scripts were in the population
// on this machine every time `make wild` ran. Adding one entry would have
// fixed that sighting and left the next one: the failure was not a missing
// name, it was a list written from memory by someone who does not hold every
// convention in it.
//
// So a name is also matched by its first word, split on the separators a name
// is qualified with. That is what makes test-suite, test_data, tests.old and
// spec-helpers denied without any of them being listed, and it is the half
// that does not need anyone to have thought of them.
var foreignTestDirs = map[string]bool{
	// The word itself, and the forms that are one word rather than two: vim's
	// syntax tests use testdir, Go uses testdata, autotools projects call it
	// testsuite, and everyone else uses the plural.
	"test": true, "tests": true, "testdata": true, "testdir": true,
	"testing": true, "testsuite": true, "testsuites": true,
	"testcase": true, "testcases": true,
	// Names with no relation to the word. RSpec and shellspec use spec, and
	// shellspec is how a shell project tests shell; jest uses __tests__ and
	// __mocks__; BSD and Postgres call it regress; fixtures are a suite's
	// inputs and are the same expression as the suite.
	"spec": true, "specs": true,
	"fixture": true, "fixtures": true,
	"__tests__": true, "__mocks__": true,
	"regress": true, "regression": true,
	// Not `t`, which is Perl's and Raku's. It is also what macOS calls the
	// per-user temporary directory, so the entry denied every path under
	// $TMPDIR — including every scratch tree this package's own tests sweep,
	// which is how it was found. A name one letter long carries no evidence
	// of what it is for, and Perl's suites are not shell in any case.
}

// nameSeparators are what a directory name is qualified with, and so where its
// first word ends.
const nameSeparators = "-_."

// foreignTestDir reports whether a directory segment names another project's
// test directory.
//
// Case-folded, because the name is a convention and not an identifier: zsh
// calls its own Test, a project vendored from elsewhere carries TestData, and
// a rule exact about case would let every one of them through while looking
// like it had them covered.
//
// A word rather than a prefix, and that was measured rather than assumed. A
// prefix on the whole segment reads as the safer rule — over-skipping is the
// direction to err, since a skipped file is counted rather than lost — and it
// is not, because it does not stop at test directories. `test` as a prefix
// denies testify, which is a library; /home/testuser, which is a person; and
// every scratch directory the Go tool makes, which is named after the test
// function and so begins with Test. Landing it took out twelve of this
// package's own tests at once by denying the temporary directories they sweep,
// which is a sweep reporting zero for a reason that has nothing to do with
// what is installed — the failure this whole file exists to keep out of the
// report. A word ends at a separator; testify and testuser are one word, and
// test-suite is two.
//
// Deliberately not extended to examples. A project's examples are its own
// programs, written to be read, and CLEANROOM.md's red list names test files
// and testdata — the same line that leaves a third-party program in /usr/bin
// readable. Denying examples would cost the sweep the population it exists for
// and would not be this rule.
func foreignTestDir(seg string) bool {
	seg = strings.ToLower(seg)
	if foreignTestDirs[seg] {
		return true
	}
	// i > 0 rather than i >= 0 says a first word has to be a word. The two
	// behave alike — a name beginning with a separator has an empty first
	// word, which is in no list — and the comparison states the intent.
	if i := strings.IndexAny(seg, nameSeparators); i > 0 {
		return foreignTestDirs[seg[:i]]
	}
	return false
}

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
// own, when non-empty, is the root of the working tree the sweep runs from.
// Inside it the rules narrow rather than stop; see deniedInOurOwnTree.
func Denied(path, own string) string {
	dir := filepath.Dir(path)
	if own != "" && underDir(path, own) {
		return deniedInOurOwnTree(dir, own)
	}
	return deniedSegments(dir)
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
		return deniedInOurOwnTree(path, own)
	}
	return deniedSegments(path)
}

// deniedInOurOwnTree reports why a path inside this project's own working tree
// may still not be opened, or "" when it may be read.
//
// The two rules above are not equally ours to stand down, and until this they
// both stood down together. The test-directory rule marks *someone else's*
// suite; this project's own testdata is generated from oracle runs and is ours
// to read, which is the entire reason own exists. The shell-source rule is a
// different claim: nothing in this repository is another shell's distribution
// unless something put it there, and there is exactly one thing that would.
//
// A third-party suite that must be run is fetched at test time and never
// committed — CLEANROOM.md says so, AGENTS.md repeats it, and #498 is the
// issue that would do it. It lands under the gitignored build directory, which
// is inside the working tree, which is where the blanket exemption turned this
// guard off. Measured before it was changed: `bash-5.3/tests/case.sub` is
// refused as another shell's source tree anywhere on the machine *except*
// inside the checkout, which is the one place a fetch puts it. A rule with a
// carve-out that swallows the case it exists for reads as protection and is
// none, and this repository has lost more to a defective instrument than to
// defective code.
//
// Only the segments below own are examined. The path *above* a checkout is
// somebody's own directory names — a worktree called `bash-fix` is not bash —
// and judging those would refuse a tree for what its parent happened to be
// called.
func deniedInOurOwnTree(dir, own string) string {
	rel, err := filepath.Rel(own, dir)
	if err != nil {
		// Not expressible as a path relative to own, so the claim that it is
		// inside our tree cannot be checked. Judged as anyone else's.
		return deniedSegments(dir)
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for i, seg := range segs {
		if seg == "." {
			continue
		}
		if shellTree(segs, i) {
			return ReasonShellSource
		}
	}
	return ""
}

// deniedSegments scans a directory path for a segment that names someone
// else's tree.
func deniedSegments(dir string) string {
	segs := strings.Split(filepath.ToSlash(dir), "/")
	for i, seg := range segs {
		if shellTree(segs, i) {
			return ReasonShellSource
		}
		if foreignTestDir(seg) {
			return ReasonTestData
		}
	}
	return ""
}

// shellTree reports whether the segment at i names a shell implementation's
// own tree — an unpacked distribution by its directory name, or an installed
// one by where it sits.
//
// Factored out because it is asked in two places now and the two must not
// drift: inside this project's tree it is the only rule that still applies,
// and a copy of it there would be a second answer to one question.
func shellTree(segs []string, i int) bool {
	for _, marker := range shellTreeMarkers {
		if strings.HasPrefix(segs[i], marker) {
			return true
		}
	}
	return shellDistribution(segs, i)
}

// underDir reports whether path is dir itself or lies inside it.
func underDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
