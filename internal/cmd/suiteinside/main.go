// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command suiteinside is the half of `make suite` that runs inside a
// container image, for the one column whose reference shell exists nowhere
// else.
//
// There is no BusyBox on a stock macOS machine, so `cmd/ash` shipped measured
// by hand and graded by nothing until #2263 gave the oracle a container
// route. This is the same route, read from the same panel entry and the same
// digest-pinned image, carrying the suite instead of the corpus.
//
// It runs *both* shells in here, and that is the whole point of it existing
// as a program rather than as a docker invocation on the host. A suite file
// calls programs; grading a BusyBox run in Alpine against a `cmd/ash` run on
// macOS would score every difference between two operating systems as a
// disagreement between two shells. Both sides are in one place, on one copy
// of the files, and what grades them is [suite.Sweep] itself — cross-compiled
// from the same tree as the host half, so the scorer, the normalizer and the
// repeat check cannot drift from the ones the other four columns are measured
// with.
//
// It writes one [suite.Reply] on standard output and nothing else. Every
// failure is a reason in that envelope rather than an exit status, because
// "the reference is not BusyBox", "the suite is not in the image" and "the
// sweep broke" are three different things for the host to print and a
// non-zero exit would collapse them into one.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blairham/sh/internal/suite"
)

func main() {
	var (
		dialect = flag.String("dialect", "", "the native column to run")
		root    = flag.String("root", "/suite", "where the suite was copied to inside the image")
		bin     = flag.String("bin", "", "the dialect binary to grade, cross-compiled for this image")
		timeout = flag.Duration("timeout", suite.DefaultTimeout, "how long one file gets under one shell")
		jobs    = flag.Int("jobs", 4, "how many files run at once")
		only    = flag.String("only", "", "comma-separated file names to run")
	)
	flag.Parse()

	rep, err := sweep(*dialect, *root, *bin, *timeout, *jobs, *only)
	reply := suite.Reply{Report: rep}
	if err != nil {
		reply = suite.Reply{Err: err.Error()}
	}
	// Straight to standard output and nothing beside it: the host reads this
	// stream whole. Anything this program has to say to a person goes to
	// standard error, where the host prints it as context for a failure.
	if err := json.NewEncoder(os.Stdout).Encode(reply); err != nil {
		fmt.Fprintln(os.Stderr, "suiteinside:", err)
		os.Exit(1)
	}
}

func sweep(dialect, root, bin string, timeout time.Duration, jobs int, only string) (*suite.Report, error) {
	s, ok := suite.FindOurs(dialect)
	if !ok {
		return nil, fmt.Errorf("no native column for dialect %q", dialect)
	}
	if bin == "" {
		return nil, fmt.Errorf("no binary to grade was given for the %s column", s.Name)
	}
	if missing := s.Missing(root); len(missing) > 0 {
		return nil, fmt.Errorf("%s claims %s under %s inside the image and they are not there",
			s.Name, strings.Join(missing, ", "), root)
	}
	reference, found := suite.Locate(s.Lookup)
	if !found {
		return nil, fmt.Errorf("no %s inside the image to be the reference; looked in %s",
			s.Name, strings.Join(s.Lookup, ", "))
	}

	ctx := context.Background()
	// Identify rather than Version, which is what the host half has always
	// called: a shell that answers no version probe is identified by its
	// column's own fingerprint instead. This said Version for as long as ash
	// was the only contained column, and ash answers a version probe, so the
	// difference was invisible — until #3480 gated dash, which answers none.
	// A contained dash would then have been refused by the check below for
	// not identifying itself, and its report would have printed `could not
	// determine` beside a reference that had just been measured.
	version := s.Identify(ctx, reference)
	// Before anything is measured. A path that resolved to some other shell
	// would produce a whole healthy-looking column about the wrong binary,
	// and /bin/sh being BusyBox in one image and dash in another is exactly
	// the mislabeling this check exists for.
	if err := s.Believable(version); err != nil {
		return nil, fmt.Errorf("%s at %s: %w", s.Name, reference, err)
	}

	rep, err := suite.Sweep(ctx, s, root, bin, reference, suite.Options{
		Timeout: timeout,
		Jobs:    jobs,
		Only:    names(only),
	})
	if err != nil {
		return nil, err
	}
	rep.ReferenceVersion = version
	return &rep, nil
}

func names(list string) map[string]bool {
	if list == "" {
		return nil
	}
	set := map[string]bool{}
	for _, n := range strings.Split(list, ",") {
		if n = strings.TrimSpace(n); n != "" {
			set[n] = true
		}
	}
	return set
}
