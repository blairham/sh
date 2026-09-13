// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command suiteguard fails when our own conformance suite has lost a file or
// shrunk one.
//
// share/suite is ten files of assertions and four native dialect columns, and
// an unrelated pull request deleted all of it without a single test failing
// (#2600). This compares the tree against the merge base and says so.
//
// See internal/suiteguard for why the baseline is git history, why a file is
// the unit, and why rule two is a count where the corpus guard's is a set.
//
//	suite-guard                # compare against the merge base with main
//	go run ./internal/cmd/suiteguard -base <sha>
package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/blairham/sh/internal/suite"
	"github.com/blairham/sh/internal/suiteguard"
)

func main() {
	base := flag.String("base", "", "commit to compare the suite against; the merge base with main if it does not resolve")
	dir := flag.String("C", ".", "repository to check")
	flag.Parse()

	runs, err := runnable(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suite-guard: %v\n", err)
		os.Exit(2)
	}

	r, err := suiteguard.Check(*dir, *base, runs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suite-guard: %v\n", err)
		os.Exit(2)
	}
	short := r.Base[:min(12, len(r.Base))]
	for _, p := range r.Stale {
		fmt.Printf("suite-guard: %q is in suiteguard.Excused and no longer needs to be — the file is back and no shorter\n", p)
	}
	if r.OK() {
		fmt.Printf("suite-guard: %d files, none lost or shortened since %s (%d there)\n",
			r.HeadFiles, short, r.BaseFiles)
		return
	}
	for _, p := range r.Dropped {
		fmt.Fprintf(os.Stderr, "suite-guard: %s was present at %s and is gone\n", p, short)
	}
	for _, s := range r.Shrunk {
		fmt.Fprintf(os.Stderr, "suite-guard: %s held %d runnable lines at %s and holds %d\n",
			s.Path, s.Was, short, s.Now)
	}
	fmt.Fprintf(os.Stderr, "\nOur own suite is the only route to a column for the four dialects whose\n"+
		"own suites we can never run. A merge or a rebase that resolved a file by\n"+
		"taking one side is how this happens, and it happened: #2363 deleted all\n"+
		"862 lines of %s while adding nothing to it, and nothing failed.\n"+
		"Recover from the base revision and keep both sides; if a case is being\n"+
		"retired on purpose, say so in suiteguard.Excused.\n", suiteguard.Root)
	os.Exit(1)
}

// runnable is every file the native columns and the cross-check would run,
// asked of the instrument rather than of the disk layout.
//
// It goes through suite.Files and suite.Tiers — the same two the run itself
// goes through — so that the answer is what would actually be graded. That is
// the whole value of it as a cross-check: a walk and a run that disagree mean
// one of them is reading a suite that is not there.
func runnable(dir string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(d string) error {
		rel := path.Join(suiteguard.Root, d)
		names, err := suite.Files(filepath.Join(dir, filepath.FromSlash(rel)), suite.OurExt)
		if err != nil {
			return fmt.Errorf("%s: %w\n  a directory the native columns claim is not in the tree. "+
				"If the suite was removed, that is precisely the loss this guard is for: "+
				"recover it from the base revision", rel, err)
		}
		for _, n := range names {
			p := path.Join(suiteguard.Root, d, n)
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
		return nil
	}
	for _, t := range suite.Tiers {
		if err := add(t); err != nil {
			return nil, err
		}
	}
	for _, s := range suite.OurColumns() {
		for _, d := range s.Dirs {
			if err := add(d); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
