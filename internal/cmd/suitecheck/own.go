// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/blairham/sh/internal/suite"
)

// binSet is the -bin flag in -own mode: dialect=path, repeated.
//
// Repeated rather than one binary, because every column runs at once. The
// whole point of our own suite is that four of the five dialects can never
// get a column from somebody else's work, and an instrument invoked one
// dialect at a time would let four of them go unrun without anybody noticing.
type binSet map[string]string

func (b binSet) String() string {
	var pairs []string
	for d, p := range b {
		pairs = append(pairs, d+"="+p)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func (b binSet) Set(v string) error {
	dialect, path, ok := strings.Cut(v, "=")
	if !ok || dialect == "" || path == "" {
		return fmt.Errorf("want dialect=path, got %q", v)
	}
	b[dialect] = path
	return nil
}

// runOwn grades every native column and then asks whether the references
// agree about core/.
func runOwn(ctx context.Context, root string, bins binSet, timeout time.Duration, jobs int, only string) int {
	fmt.Printf("our own suite — %s, committed and readable\n", root)
	fmt.Println("  The reference shell on this machine is the expectation; no expected output")
	fmt.Println("  is checked in. A .right file of ours would let us record our own bug as")
	fmt.Println("  correct, which is the one failure the oracle exists to prevent.")
	fmt.Println()

	opts := suite.Options{Timeout: timeout, Jobs: jobs, Only: names(only)}
	code := 0
	var refs []suite.Reference
	var reports []suite.Report
	var skipped []string

	for _, s := range suite.OurColumns() {
		if s.NotYet != "" {
			skipped = append(skipped, fmt.Sprintf("%s — %s", s.Name, s.NotYet))
			continue
		}
		reference, found := suite.Locate(s.Lookup)
		if !found {
			skipped = append(skipped, fmt.Sprintf("%s — no %s on this machine to be the reference; looked in %s",
				s.Name, s.Name, strings.Join(s.Lookup, ", ")))
			continue
		}
		bin, ok := bins[s.Dialect]
		if !ok {
			skipped = append(skipped, fmt.Sprintf("%s — no -bin %s=… was given, so cmd/%s was not graded",
				s.Name, s.Dialect, s.Dialect))
			continue
		}
		if missing := s.Missing(root); len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "suitecheck: %s claims %s under %s and they are not there\n",
				s.Name, strings.Join(missing, ", "), root)
			code = 1
			continue
		}
		refs = append(refs, suite.Reference{Name: s.Name, Path: reference})
		rep, err := suite.Sweep(ctx, s, root, bin, reference, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suitecheck: %s: %v\n", s.Name, err)
			code = 1
			continue
		}
		rep.ReferenceVersion = suite.Version(ctx, reference)
		reports = append(reports, rep)
	}

	for _, rep := range reports {
		printOwnColumn(rep)
	}
	printOwnTable(reports)
	fmt.Println("  The tiers are claims, not filing. core/ is what a script may assume in any")
	fmt.Println("  of these shells; ext/ is this substrate's core language beyond POSIX —")
	fmt.Println("  arrays, [[ ]], $'…', +=, substrings — which dash and ash are the measured")
	fmt.Println("  holdouts on; <shell>/ is the answer only that shell has. A cross-check runs")
	fmt.Println("  a tier through the references alone and asks whether they agree, which is")
	fmt.Println("  the half no fetched suite can have: grading our binary against one reference")
	fmt.Println("  proves that dialect right, while the references agreeing proves the")
	fmt.Println("  construct common — the claim docs/spec/shell-matrix.md makes in prose.")
	fmt.Println()

	for _, name := range suite.Tiers {
		cross, err := suite.CrossCheck(ctx, root, name, refs, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suitecheck: the %s cross-check: %v\n", name, err)
			code = 1
			continue
		}
		printCross(cross)
	}

	if len(skipped) > 0 {
		fmt.Println("  columns not run")
		for _, why := range skipped {
			fmt.Printf("    %s\n", wrap(why, "      "))
		}
		fmt.Println()
		fmt.Println("  A column that is not run is printed rather than dropped: a missing column")
		fmt.Println("  that says nothing reads as a column that passed.")
		fmt.Println()
	}
	return code
}

// printOwnColumn is one native column, and it names the cases that failed.
//
// Naming them is the difference between this and the fetched columns, and it
// is the only difference in what may be printed. These files are ours.
func printOwnColumn(rep suite.Report) {
	s := rep.Suite
	fmt.Printf("%s — %d files (%s)\n", s.Name, rep.Files, strings.Join(s.Dirs, "/, ")+"/")
	fmt.Printf("  reference  %s — %s\n", rep.Reference, rep.ReferenceVersion)
	fmt.Printf("  ours       %s\n", rep.Ours)
	fmt.Printf("  parsed     %-9s %5.1f%%   our parser read the whole file\n",
		fmt.Sprintf("%d/%d", rep.Parsed, rep.Files), 100*rep.ParseRate())
	fmt.Printf("  strict     %-9s %5.1f%%   every byte and the status identical\n",
		fmt.Sprintf("%d/%d", rep.Strict, rep.Scored), 100*rep.StrictRate())
	fmt.Printf("  lines      %-9s %5.1f%%   longest common subsequence, by line\n",
		"", 100*rep.LineRate())
	if rep.Unstable+rep.OracleHung+rep.DialectHung > 0 {
		fmt.Printf("  not scored %d unstable · %d reference hung · %d ours hung\n",
			rep.Unstable, rep.OracleHung, rep.DialectHung)
	}
	failed := rep.NotStrict()
	if len(failed) == 0 {
		fmt.Println("  every case agreed")
		fmt.Println()
		return
	}
	fmt.Println("  cases to fix")
	for _, c := range failed {
		switch {
		case !c.Result.Parsed:
			fmt.Printf("    %-28s our parser refused it: %s\n", c.Name, c.Result.Cause)
		case c.Result.DialectHung:
			fmt.Printf("    %-28s ours never finished — a hang we published\n", c.Name)
		case c.Result.OracleHung:
			fmt.Printf("    %-28s the reference never finished — a harness fault, not a finding\n", c.Name)
		case c.Result.Unstable:
			fmt.Printf("    %-28s the reference would not repeat it — a pid, a clock, an order\n", c.Name)
		default:
			fmt.Printf("    %-28s %d/%d lines · status %d, reference %d\n",
				c.Name, c.Result.Common, c.Result.Longest, c.Result.OurStatus, c.Result.RefStatus)
		}
	}
	fmt.Println()
}

// printOwnTable is the per-dialect baseline, on one screen.
//
// The number this campaign burns down is strict, per column. It is the harsh
// one on purpose: a case file is a few dozen assertions and one disagreement
// forfeits all of them, which is the right pressure for a suite we control —
// unlike a fetched file of hundreds, where it reads as catastrophe.
func printOwnTable(reports []suite.Report) {
	if len(reports) == 0 {
		return
	}
	fmt.Println("  per-dialect baseline")
	fmt.Printf("    %-8s %-9s %-9s %s\n", "column", "parsed", "strict", "lines")
	for _, rep := range reports {
		fmt.Printf("    %-8s %-9s %-9s %5.1f%%\n",
			rep.Suite.Name,
			fmt.Sprintf("%d/%d", rep.Parsed, rep.Files),
			fmt.Sprintf("%d/%d", rep.Strict, rep.Scored),
			100*rep.LineRate())
	}
	fmt.Println()
}

// printCross is the core claim, measured.
func printCross(cross suite.Cross) {
	if len(cross.Shells) < 2 {
		return
	}
	fmt.Printf("  %s/ agreement  %d/%d   the reference shells themselves wrote the same bytes\n",
		cross.Tier, cross.Agree, cross.Files)
	fmt.Printf("                  across %s\n", strings.Join(cross.Shells, ", "))
	for _, split := range cross.Split {
		var groups []string
		for _, g := range split.Groups {
			groups = append(groups, strings.Join(g, "+"))
		}
		fmt.Printf("    %-28s not core: %s\n", split.Name, strings.Join(groups, " ≠ "))
	}
	for _, name := range cross.Unstable {
		fmt.Printf("    %-28s a reference would not repeat it, so it says nothing either way\n", name)
	}
	fmt.Println()
}
