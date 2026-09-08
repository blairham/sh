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

// Temp gives a run a temporary directory of its own and fails it if anything
// is left there. A package guards itself with
//
//	func TestMain(m *testing.M) { os.Exit(treeguard.Temp(m).Run()) }
//
// and composes it with the other wrappers the way childguard.Wrap composes.
//
// # Why this is not Run with a different directory
//
// Run counts *files*, and says so: a directory a test makes and empties again
// has left nothing behind, and reporting it would make the guard cry wolf in
// a source tree where git does not record an empty directory anyway.
//
// The leak this is for is an empty directory and nothing else. A process
// substitution makes one under the shell's TMPDIR to hold its named pipes;
// the pipes go as soon as the command that named them ends, and for a long
// time nothing removed what was left. 10,585 accumulated in /tmp in one
// working session and 4,188 more in the two days after they were swept by
// hand (#1284). Run, pointed at TMPDIR, would have reported none of them —
// which is the point worth writing down: the guard that was already here
// could not have caught this, so the answer was to teach it the case rather
// than to leave a second guard somewhere else that knows one thing this one
// does not.
//
// So an entry of any kind counts here, and a directory is named rather than
// descended into: what is inside a stray is the writer's business, and the
// report wants one line per thing left behind.
//
// # What it covers
//
// A whole run, not a test. It cannot say which test leaked — the run is over
// by the time it looks — but a package that starts shells is a small enough
// haystack, and turning a silent success into a failed package is the whole
// point. The suites that start shells are where a leak reaches a *binary*:
// interp's own tests each hand their Runner a TMPDIR the framework takes away
// again, so a leak there is invisible, and a leak in a shipped shell is
// permanent.
//
// A run killed by a signal leaves the scratch directory behind, and a
// SIGKILLed one cannot do otherwise. That is the same hole every cleanup has
// and it is not worth pretending about.
func Temp(m interface{ Run() int }) interface{ Run() int } { return tempGuard{m} }

type tempGuard struct{ inner interface{ Run() int } }

func (g tempGuard) Run() int {
	dir, err := os.MkdirTemp("", "sh-tempguard-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "treeguard: no scratch temporary directory: %v — not guarding\n", err)
		return g.inner.Run()
	}
	// Restored rather than left pointing at a directory that is about to be
	// removed: a wrapper outside this one goes on running afterwards.
	was, had := os.LookupEnv("TMPDIR")
	if err := os.Setenv("TMPDIR", dir); err != nil {
		fmt.Fprintf(os.Stderr, "treeguard: %v — not guarding\n", err)
		_ = os.RemoveAll(dir)
		return g.inner.Run()
	}

	code := g.inner.Run()

	if had {
		_ = os.Setenv("TMPDIR", was)
	} else {
		_ = os.Unsetenv("TMPDIR")
	}
	// Read before the removal, and the removal happens either way: a leaked
	// scratch directory is a worse second failure than the first one.
	left := strays(dir)
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "treeguard: scratch temporary directory not removed: %v\n", err)
	}
	if len(left) == 0 {
		return code
	}
	fmt.Fprintf(os.Stderr, "\ntreeguard: the tests left %d entr(ies) in their temporary directory:\n", len(left))
	for _, p := range left {
		fmt.Fprintf(os.Stderr, "\t%s\n", p)
	}
	fmt.Fprint(os.Stderr, "A shell that ends removes what it made for itself — the directory a "+
		"process substitution holds its pipes in, above all. One left here is one left in "+
		"/tmp on a real machine, where nothing ever removes it.\n")
	if code == 0 {
		return 1
	}
	return code
}

// strays names what is left at the top of dir, sorted so the report reads the
// same twice.
//
// Directories are named and not descended into, and an empty one counts: that
// is the whole difference from snapshot, and it is the case this exists for.
func strays(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("(unreadable: %v)", err)}
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, filepath.Join(dir, e.Name()))
	}
	sort.Strings(names)
	return names
}
