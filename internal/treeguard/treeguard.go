// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package treeguard fails a test binary that leaves a file behind in the
// directory it ran in.
//
// `go test` runs a package's binary in that package's *source* directory, so a
// test that writes to a relative path writes into the checked-out tree. Nothing
// reports it: the write succeeds, the test passes, and the file sits there
// until a later `git add -A` commits it into a change that never meant to
// create it. That has happened twice in this repository with one file.
//
// The guard is a before-and-after listing around the whole run. It cannot say
// which test wrote the file — the run is over by the time it looks — but naming
// the file is enough to find the writer, and turning a silent success into a
// failed package is the whole point: an unnoticed stray becomes a red build
// before it can reach anyone else's commit.
//
// What it does *not* cover is a write outside the package directory. A test
// that needs a filesystem should take a directory from the framework, which
// takes it away again; this exists for the case where nobody remembered to.
package treeguard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Run runs the package's tests and returns the status the binary should exit
// with. A package guards itself with
//
//	func TestMain(m *testing.M) { os.Exit(treeguard.Run(m)) }
//
// and the status is m.Run's unless the run left something behind, in which case
// a passing package still fails.
//
// The parameter is the one method *testing.M offers here rather than the type
// itself, so this package's own tests can hand it a run that leaves a file —
// which is the case that matters and the one a real *testing.M cannot be asked
// to perform.
func Run(m interface{ Run() int }) int {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "treeguard: %v\n", err)
		return 1
	}
	// A snapshot that cannot be taken is reported and not enforced: refusing
	// to run the tests over it would trade a real suite for a hygiene check.
	before, err := snapshot(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "treeguard: %v — not guarding\n", err)
		return m.Run()
	}

	code := m.Run()

	after, err := snapshot(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "treeguard: %v\n", err)
		return code
	}
	added := addedPaths(before, after)
	if len(added) == 0 {
		return code
	}
	fmt.Fprintf(os.Stderr, "\ntreeguard: the tests left %d file(s) in the source tree:\n", len(added))
	for _, p := range added {
		fmt.Fprintf(os.Stderr, "\t%s\n", filepath.Join(dir, p))
	}
	fmt.Fprint(os.Stderr, "A test that writes to a relative path writes here, "+
		"because that is where `go test` runs it. Give the writer a directory "+
		"the framework takes away again, then delete these before committing.\n")
	if code == 0 {
		return 1
	}
	return code
}

// snapshot names every file under dir, relative to it.
//
// A directory is recorded by its entries rather than by itself, so a directory
// a test makes and empties again is not reported: what matters is whether
// anything is left, not whether something was there in between.
func snapshot(dir string) (map[string]bool, error) {
	found := make(map[string]bool)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A file that vanished under the walk is not a file that was
			// left behind, which is the only thing being counted.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		found[rel] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// addedPaths is what the second snapshot has and the first did not, sorted so
// the report reads the same twice.
func addedPaths(before, after map[string]bool) []string {
	var added []string
	for p := range after {
		if !before[p] {
			added = append(added, p)
		}
	}
	sort.Strings(added)
	return added
}
