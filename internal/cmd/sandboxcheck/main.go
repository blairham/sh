// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command sandboxcheck grades the boundary from outside the process: it runs
// the shell that ships, tries every way a script has of reaching the
// filesystem, and asks the filesystem what happened.
//
// Run it with `make sandbox`. Every route is run three times — with no
// policy, with one that forbids it, and with one that permits it — so a row
// can say "the gate stopped this" rather than "nothing happened", which are
// the two things a sandbox test confuses.
//
// Unlike the other report targets, this one **exits non-zero** when it finds
// an escape. A conformance number is meant to be low and climbing; a
// boundary is meant to hold, so a hole in it is a failure rather than a
// score.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/internal/sandboxcheck"
)

// dialectBins collects `-dialect-bin <name>=<path>`, which is repeatable.
//
// Repeatable rather than a separated list, for the reason -deny is: every
// separator that would do is a character a path may legally contain.
type dialectBins map[string]string

func (b dialectBins) String() string {
	var out []string
	for d, path := range b {
		out = append(out, d+"="+path)
	}
	return strings.Join(out, " ")
}

func (b dialectBins) Set(v string) error {
	d, path, ok := strings.Cut(v, "=")
	if !ok || d == "" || path == "" {
		return fmt.Errorf("want <dialect>=<path>, got %q", v)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	b[d] = abs
	return nil
}

func main() {
	var (
		bin  = flag.String("bin", "", "the shell binary to grade")
		root = flag.String("dir", "", "where to make the scratch directories")
		only = flag.String("run", "", "grade only the routes whose name contains this")
		verb = flag.Bool("v", false, "print the three runs for every route, not only the ones that need explaining")
		bins = dialectBins{}
	)
	flag.Var(bins, "dialect-bin", "also grade <dialect>=<path> through the binary's own --policy; repeatable")
	flag.Parse()

	if *bin == "" {
		fmt.Fprintln(os.Stderr, "sandboxcheck: name the binary to grade with -bin")
		os.Exit(2)
	}
	// Absolute, because every route runs from a workspace of its own and a
	// relative path names nothing from there — a failure that would read as
	// a shell which cannot start.
	shell, err := filepath.Abs(*bin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sandboxcheck:", err)
		os.Exit(2)
	}
	dir := *root
	if dir == "" {
		dir = filepath.Join(filepath.Dir(shell), "sandboxcheck")
	}
	// Absolute for the same reason the binary is, and it is not a
	// formality: every script runs with the workspace as its working
	// directory, so a relative root makes each fixture path name something
	// under the workspace that is not there. The sweep then reports every
	// row inert — which is the instrument declining to call an unreachable
	// route "contained", and is exactly what a silently mis-pointed
	// grader should look like rather than a green table.
	if dir, err = filepath.Abs(dir); err != nil {
		fmt.Fprintln(os.Stderr, "sandboxcheck:", err)
		os.Exit(2)
	}
	// Beside the binary rather than in a temporary directory, and that is
	// the one setting here worth arguing about. Anything that carves out
	// TMPDIR for commands to work in exempts the very thing under test, and
	// the failure is silent: the workspace is permitted for a reason that
	// has nothing to do with the policy, and every route reports contained.
	// Nothing in this shell carves out TMPDIR today, and the instrument that
	// would prove it must not be the one that breaks quietly if something
	// starts to.
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "sandboxcheck:", err)
		os.Exit(2)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	rep, err := sandboxcheck.Run(shell, dir, *only)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sandboxcheck:", err)
		os.Exit(2)
	}
	fmt.Print(rep.Text(*verb))
	failed := rep.Failed()

	// The same routes again, against the binaries somebody actually invokes,
	// through the `--policy` each one carries since #1826.
	//
	// A second table rather than a wider one, because they are two answers to
	// two questions: the first says the gate holds, and this says the *flag*
	// reaches it on the binary a shebang, `chsh` and `login` name. Before
	// #1826 the second had no flag at all and the first table was green
	// throughout — which is the same shape as the `-c` drift that had
	// `make conformance-dialects` grading the drivers rather than the
	// dialects, and is why this is a sweep rather than a unit test.
	if len(bins) > 0 {
		// A scratch root of its own, for the reason each run gets a fresh
		// fixture: a sweep measuring what the previous one left behind is
		// measuring the leftovers.
		binDir := dir + "-bins"
		if err := os.RemoveAll(binDir); err != nil {
			fmt.Fprintln(os.Stderr, "sandboxcheck:", err)
			os.Exit(2)
		}
		defer func() { _ = os.RemoveAll(binDir) }()
		fmt.Println()
		rep, err := sandboxcheck.RunBinaries(bins, binDir, *only)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sandboxcheck:", err)
			os.Exit(2)
		}
		fmt.Print(rep.Text(*verb))
		failed = failed || rep.Failed()
	}
	if failed {
		os.Exit(1)
	}
}
