// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package suiteguard checks that our own conformance suite never loses a file
// or shrinks one.
//
// It exists because it already happened. PR #2356 landed share/suite — 862
// lines across ten files, and the four native dialect columns that are the
// only route this project has to per-dialect parity at scale. PR #2363, about
// alias options and `-sc`, deleted every line of it: zero lines added under
// share/suite, 862 removed, the `suite` target gone with them, and nothing in
// the deleted text about aliases or `-sc`. Nothing failed. `make check` was
// green, every test passed, and the loss was found days later by somebody
// asking why `make suite` was not a target (#2600).
//
// internal/corpusguard is the same idea over the oracle corpus, and it is why
// no corpus row has been lost since it landed. share/suite had no equivalent,
// which is the whole reason there was a hole here to fall into.
//
// # What is a "case" here
//
// The corpus is a set of named cases, so its guard compares IDs. A suite file
// has no names in it — it is a script of assertions a shell runs from the
// top, and the unit the instrument scores is the *file*: the report says
// 10/10 strict over ten files. So the file is the case, and rule one is a set
// difference over paths, exactly as corpusguard's is over IDs.
//
// That alone would not have caught the sibling hazard, though. The mechanism
// both guards are built for is a merge or a rebase resolving a file by taking
// one side, and taking one side *of a suite file* leaves its path in place
// while reverting its contents. There are no IDs inside to difference, so
// rule two is the weaker question a file of assertions can still answer: a
// file may not have fewer runnable lines than it had at the base. Suite files
// only grow — a case that agreed with the reference shell goes on agreeing,
// so an existing assertion is corrected in place rather than dropped — and a
// fall is therefore evidence rather than noise.
//
// # Why the baseline is git history
//
// For corpusguard's reason, which is worth restating because it is the part
// people try to simplify away: a committed manifest of file names is another
// text file in the same tree, merged by the same merge, and a merge that
// deletes share/suite is in a position to delete its manifest in the same
// commit. A guard whose baseline the guarded event can edit is not a guard.
// Git history is the one baseline a working tree cannot rewrite, and it needs
// no maintenance, so it cannot go stale.
//
// # Why a count is enough here and was not enough there
//
// corpusguard argues against counts, and the argument holds: a branch that
// adds two cases and merges two away leaves a total unchanged. Rule one is a
// set for that reason. Rule two is a count, but it is a count *per file* and
// against that file's own base, so the offsetting move that defeats a total
// is not available to it — five lines added to quoting.tests do not conceal
// four reverted out of redirect.tests. Within one file it is genuinely
// weaker than a set difference would be, and there is no set to difference;
// that is a limit of the data, recorded here rather than papered over.
package suiteguard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/internal/corpusguard"
)

// Root is our own suite, relative to the repository root. It is spelled here
// rather than taken from internal/suite so that this package does not pull in
// every dialect; the command cross-checks the two against each other on every
// run, which is stronger than sharing a constant.
const Root = "share/suite"

// Excused names files that may be gone, or shorter than they were, with the
// reason. It is the deliberate exception, and the only one.
//
// Empty, and it should stay that way. A suite case is a question a real shell
// has already answered; retiring one throws away a measurement rather than a
// line of code. Losing an entry from this map is fail-safe — the guard starts
// reporting the file again — which is the direction a merge hazard should
// point.
var Excused = map[string]string{}

// File is one file of the suite as the baseline saw it.
type File struct {
	// Path is relative to the repository root.
	Path string
	// Lines is the runnable lines: neither blank nor a comment. It is what
	// rule two compares, and it is deliberately not the byte count — a
	// reflowed comment is not a lost assertion.
	Lines int
}

// Shrunk is one file that still exists and holds less than it did.
type Shrunk struct {
	Path     string
	Was, Now int
}

// Result is what one comparison found.
type Result struct {
	// Base is the resolved commit the suite was compared against.
	Base string
	// BaseFiles and HeadFiles are the suite's size at each end.
	BaseFiles, HeadFiles int
	// Dropped are paths present at Base, absent now, and not excused. A
	// non-empty Dropped is the failure this package exists for.
	Dropped []string
	// Shrunk are files that survived with fewer runnable lines than they had.
	Shrunk []Shrunk
	// Stale are excuses that are no longer needed: the file is back, and at
	// least as long as it was. Reported, never fatal — an excuse written on
	// another branch is not this branch's business to fail on.
	Stale []string
}

// OK reports whether nothing left and nothing shrank.
func (r *Result) OK() bool { return len(r.Dropped) == 0 && len(r.Shrunk) == 0 }

// Lines counts the runnable lines of a suite file: neither blank nor a
// comment. A comment is a line whose first non-space byte is '#', which is
// every comment a shell script has.
func Lines(src []byte) int {
	n := 0
	for _, line := range strings.Split(string(src), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		n++
	}
	return n
}

// Walk is every regular file under Root in dir's working tree, by path
// relative to the repository root, with its runnable lines counted.
//
// Every file and not only the ones that run: a README under share/suite is
// not a case, but a merge that takes it away took the directory away, and the
// point of rule one is that a path may not silently leave.
func Walk(dir string) ([]File, error) {
	root := filepath.Join(dir, filepath.FromSlash(Root))
	var files []File
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Lines: Lines(src)})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// at is the suite as it was at one commit.
func at(dir, base string) ([]File, error) {
	out, err := corpusguard.Git(dir, "ls-tree", "-r", "--name-only", base, "--", Root)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, path := range strings.Split(out, "\n") {
		if path == "" {
			continue
		}
		src, err := corpusguard.Git(dir, "show", base+":"+path)
		if err != nil {
			return nil, err
		}
		// git's output is trimmed of its trailing newline, which cannot
		// change a count of non-blank lines.
		files = append(files, File{Path: path, Lines: Lines([]byte(src))})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Compare reports what base has that head does not, and what head still has
// but holds less of.
func Compare(base, head []File) *Result {
	now := map[string]int{}
	for _, f := range head {
		now[f.Path] = f.Lines
	}
	r := &Result{BaseFiles: len(base), HeadFiles: len(head)}
	for _, f := range base {
		lines, here := now[f.Path]
		switch {
		case !here && Excused[f.Path] == "":
			r.Dropped = append(r.Dropped, f.Path)
		case !here:
			// excused away
		case lines < f.Lines && Excused[f.Path] == "":
			r.Shrunk = append(r.Shrunk, Shrunk{Path: f.Path, Was: f.Lines, Now: lines})
		case lines >= f.Lines && Excused[f.Path] != "":
			r.Stale = append(r.Stale, f.Path)
		}
	}
	sort.Strings(r.Dropped)
	sort.Strings(r.Stale)
	sort.Slice(r.Shrunk, func(i, j int) bool { return r.Shrunk[i].Path < r.Shrunk[j].Path })
	return r
}

// Check compares the suite in dir's working tree against the suite at base.
//
// runs is the files the compiled instrument would actually run, by path
// relative to the repository root. Checking the walk against it is what makes
// the walk credible, for corpusguard's reason: the broken form of a file
// scanner is silence, and silence here reads as "nothing was lost". A walk
// that found no files because share/suite moved, or a tier the instrument
// claims and the tree does not have, would otherwise pass this guard on the
// day the suite disappeared — which is the exact day it must not.
func Check(dir, prefer string, runs []string) (*Result, error) {
	head, err := Walk(dir)
	if err != nil {
		return nil, err
	}
	if err := agree(head, runs); err != nil {
		return nil, err
	}
	base, err := corpusguard.ResolveBase(dir, prefer)
	if err != nil {
		return nil, err
	}
	was, err := at(dir, base)
	if err != nil {
		return nil, err
	}
	r := Compare(was, head)
	r.Base = base
	return r, nil
}

// agree reports whether the walk and the instrument name the same runnable
// files. Extension is how the two are reconciled: the walk takes everything
// under Root, the instrument takes the files its columns run, and the
// difference between them may only be files that do not run.
func agree(head []File, runs []string) error {
	want := map[string]bool{}
	for _, p := range runs {
		want[p] = true
	}
	for _, f := range head {
		delete(want, f.Path)
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for p := range want {
			missing = append(missing, p)
		}
		sort.Strings(missing)
		return fmt.Errorf("the instrument runs %s, which the walk of %s did not find; "+
			"the walk no longer reads this suite's shape and cannot be trusted about history",
			strings.Join(missing, ", "), Root)
	}
	if len(runs) == 0 {
		return fmt.Errorf("the instrument runs no files at all under %s; "+
			"a guard that compares an empty suite against history reports nothing lost "+
			"on the day the suite is lost", Root)
	}
	return nil
}
