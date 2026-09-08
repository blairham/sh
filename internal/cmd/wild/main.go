// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command wild parses the shell scripts installed on this machine and reports
// the ones this parser cannot read.
//
// It reads and never runs: the reference shell is consulted with `-n`, and
// nothing here executes a script. That is what makes it safe to point at
// /usr/bin.
//
// It is a report and not a gate, for the same reason `make conformance` is
// one: the answer depends on what happens to be installed, so it cannot be
// the same twice on two machines. What it is good at is finding the questions
// nobody thought to put in the corpus.
//
// It sweeps two populations and names both. The installed programs under
// /bin, /etc and the package manager's trees are what -dirs holds; the plugin
// and framework trees a shell sources at startup are configured in
// SH_WILD_DIRS, because their location is one machine's and no default could
// be right. The second is where every daily-driver construct found so far
// lived, and its absence is printed rather than passed over: "0 failures" over
// a population that was never looked at is the reading this report has to make
// impossible.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/wild"
	"github.com/blairham/sh/syntax"
)

func main() {
	var (
		dirs = flag.String("dirs", strings.Join(wild.DefaultDirs, ","),
			"comma-separated directories of installed programs to sweep; "+wild.DirsVar+" adds the framework trees a shell sources at startup")
		which = flag.String("dialect", "bash",
			"which dialect to grade, and so which shebangs count: bash or zsh")
		reference = flag.String("reference", "",
			"shell that decides whether a refused file is really a shell script; defaults to the dialect's own, empty to trust every shebang")
		verbose = flag.Bool("v", false, "list every failure's message in full")
		depth   = flag.Int("depth", wild.DefaultDepth,
			"how far below each directory to descend; a negative number reads the directories themselves and nothing under them")
		list = flag.Bool("list", false,
			"print the scripts the sweep would read, one per line, and stop")
		run = flag.String("run", "",
			"also RUN each script that parses, under this binary and the reference, and report where they disagree")
		runargs = flag.String("runargs", "",
			"space-separated flags the -run binary needs, before the script")
		contained = flag.Bool("contained", false,
			"hand the -run binary a policy allowing it to write only in the directory each run is given, so the sweep tests the boundary rather than the arrangement")
		timeout = flag.Duration("timeout", 10*time.Second, "how long one run may take")
	)
	flag.Parse()
	dialect, shells, byDefault, err := chosen(*which)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wild:", err)
		os.Exit(2)
	}
	// An unset reference means the dialect's own shell, which is the only one
	// whose refusal says anything: asking bash whether a zsh script is shell
	// answers a different question, and answers it wrong.
	if !isSet("reference") {
		*reference = byDefault
	}
	// Resolved to a path before it is used, so a diagnostic naming the shell
	// can be recognized and so normalise is never handed a bare word. A name
	// that is not on PATH is left as it was and fails where it is used, which
	// says more than a failure here would.
	if p, err := exec.LookPath(*reference); err == nil {
		*reference = p
	}

	installed := wild.Scope{Dirs: strings.Split(*dirs, ","), Shells: shells, Depth: *depth}
	frameworks := wild.Scope{Dirs: wild.DirsFrom(os.LookupEnv), Shells: shells, Depth: frameworkDepth(*depth)}
	if *list {
		// The run sweep executes third-party code, and the only responsible
		// way to choose what it runs is to read the scripts first. Nothing
		// else prints the population, so choosing a subset meant guessing at
		// it — which is how a sweep ends up running something nobody looked at.
		for _, scope := range []wild.Scope{installed, frameworks} {
			paths, _ := wild.Find(scope)
			for _, p := range paths {
				fmt.Println(p)
			}
		}
		return
	}

	// Absent roots are announced only where somebody named them. The default
	// list deliberately holds directories a given machine will not have — one
	// list serving a mac and a container is the point of it — so reporting
	// those every run would train a reader to skip the line that matters.
	rep := sweep(installedPopulation, installed, dialect, *reference, *verbose, isSet("dirs"))
	sweep(frameworkPopulation, frameworks, dialect, *reference, *verbose, true)

	if *run != "" {
		under := wild.UnderTest{Path: *run, Args: strings.Fields(*runargs), Contained: *contained}
		// The installed population only. A framework file is *sourced* — it
		// defines a function or sets an option for the shell that reads it —
		// so running one with `--version` executes somebody's startup code to
		// learn nothing, and the probes this sweep is built on assume a
		// program that was meant to be invoked.
		runSweep(under, *reference, installed, rep, *timeout, *verbose)
	}
	// A report rather than a gate: the count is the point, and failing here
	// would fail on whichever machine happens to have the oddest scripts.
	_ = os.Stdout.Sync()
}

// The two populations, named because the difference between them is what this
// sweep was read as answering and did not.
//
// A bin directory holds programs a person runs; a framework tree holds the
// code a person's shell reads before it draws a prompt. `make wild` reported
// "0 failures" on a day a real plugin tree held twenty-two parse failures over
// three bugs, and the number was not wrong — it was a regression guard over
// one population being quoted as coverage of the other. Both are named on
// every run now, including the one that was not swept, because a population
// nobody mentions is the one a reader assumes was included.
const (
	installedPopulation = "installed programs"
	frameworkPopulation = "framework trees"
)

// sweep runs one population and prints it, and returns the report so a run
// sweep can skip what already failed to parse.
//
// A population with no roots prints what it would take to have some rather
// than nothing at all. Silence is what let "0 failures" be read as "nothing on
// this machine fails".
func sweep(name string, scope wild.Scope, dialect syntax.Dialect, reference string, verbose, named bool) wild.Report {
	if len(scope.Dirs) == 0 {
		fmt.Printf("%s: not swept — no roots configured. Set %s to the plugin or framework trees a shell sources at startup, separated like PATH.\n",
			name, wild.DirsVar)
		return wild.Report{}
	}
	// An absent root is skipped rather than fatal, so one variable serves a
	// laptop and a CI runner — but it is said out loud, because the thing a
	// silent skip hides is a typo, and a typo here reads as a clean sweep.
	if absent := wild.Absent(scope.Dirs); named && len(absent) > 0 {
		fmt.Printf("%s: %d of %d roots are not directories here and were skipped: %s\n",
			name, len(absent), len(scope.Dirs), strings.Join(absent, " "))
	}
	rep := wild.Sweep(context.Background(), scope, dialect, reference)
	fmt.Printf("%s: scripts found: %d   parsed: %d   refused by %s too: %d   failures: %d\n",
		name, rep.Scanned, rep.Parsed, refName(reference), rep.NotShell, len(rep.Failures))
	// Skipped files are counted and never named: printing a path would
	// invite the reader to open a file CLEANROOM.md forbids opening.
	if len(rep.Skipped) > 0 {
		fmt.Printf("%s: skipped without reading: %s — CLEANROOM.md forbids opening these, so they are counted, never listed\n",
			name, skipSummary(rep.Skipped))
	}
	report(name, rep, verbose)
	return rep
}

// frameworkDepth is how far to descend below a configured root: deeper than a
// bin directory by default, and whatever was asked for when -depth was given.
func frameworkDepth(depth int) int {
	if isSet("depth") {
		return depth
	}
	return wild.FrameworkDepth
}

// report prints the failures ranked by cause, largest first.
//
// Ranked rather than listed, because the list is the wrong artifact: a reader
// facing sixty lines cannot tell whether that is sixty problems or three, and
// the sweep is the only thing that can tell them. The paths stay available
// under -v, since a cause without an example is a category rather than a bug.
func report(name string, rep wild.Report, verbose bool) {
	causes := wild.Causes(rep.Failures)
	if len(causes) == 0 {
		return
	}
	fmt.Printf("\n%s not parsed, by cause (%d causes over %d scripts):\n", name, len(causes), len(rep.Failures))
	for _, c := range causes {
		fmt.Printf("  %4d  %s\n", c.Count(), c.Reason)
		if c.Example.Text != "" {
			fmt.Printf("        %s:%d: %s\n", c.Example.Path, c.Example.Line, c.Example.Text)
		} else {
			fmt.Printf("        %s: %s\n", c.Example.Path, firstLine(c.Example.Err.Error()))
		}
		if verbose {
			for _, p := range c.Paths {
				fmt.Printf("          %s\n", p)
			}
		}
	}
}

// runSweep executes the scripts that parse and compares the two shells.
//
// Only the ones that parse: a script this shell cannot read has already been
// reported, and running it would say the same thing twice.
func runSweep(ours wild.UnderTest, reference string, scope wild.Scope, parsed wild.Report, timeout time.Duration, verbose bool) {
	failed := map[string]bool{}
	for _, f := range parsed.Failures {
		failed[f.Path] = true
	}
	var paths []string
	found, _ := wild.Find(scope)
	for _, p := range found {
		if !failed[p] {
			paths = append(paths, p)
		}
	}

	rep := wild.RunSweep(context.Background(), paths, ours, reference, timeout)
	// Said on the line with the numbers, because the numbers mean something
	// different with it on: a disagreement under a policy may be the boundary
	// refusing rather than the two shells differing, and a reader who did not
	// know which run this was could not tell.
	posture := ""
	if ours.Contained {
		posture = "   contained: writes only in each run's own directory"
	}
	fmt.Printf("\nran: %d   agreed: %d   timed out: %d   not the same twice: %d   disagreed: %d%s\n",
		rep.Ran, rep.Agreed, rep.Timedout, rep.Unstable, len(rep.Mismatches), posture)
	for _, m := range rep.Mismatches {
		fmt.Printf("  %s %s\n", m.Path, strings.Join(m.Args, " "))
		fmt.Printf("    ours   (%d) %s\n", m.OurStatus, show(m.Ours, verbose))
		fmt.Printf("    theirs (%d) %s\n", m.TheirStatus, show(m.Theirs, verbose))
	}
}

// show renders a run's output for a report: one line unless asked for more,
// because a script's whole output is rarely what tells the two apart.
func show(out string, verbose bool) string {
	out = strings.TrimRight(out, "\n")
	if verbose {
		return "\n      " + strings.ReplaceAll(out, "\n", "\n      ")
	}
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		out = out[:i] + " …"
	}
	if len(out) > 90 {
		out = out[:90] + " …"
	}
	return out
}

// skipSummary renders the skip counts as "N in <reason>, M in <reason>", in
// a stable order.
func skipSummary(skipped map[string]int) string {
	reasons := make([]string, 0, len(skipped))
	for r := range skipped {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		parts = append(parts, fmt.Sprintf("%d in %s", skipped[r], r))
	}
	return strings.Join(parts, ", ")
}

// chosen turns the -dialect word into the three things that move with it: the
// grammar to parse with, the shebangs that count, and the shell to ask when we
// refuse a file.
//
// The three are one choice. A zsh sweep that kept bash's shebang set would
// grade zsh on `#!/bin/sh` scripts that never claimed to be zsh, and one that
// kept bash as the reference would let bash's opinion decide whether a zsh
// script is a shell script at all.
func chosen(name string) (syntax.Dialect, map[string]bool, string, error) {
	switch name {
	case "bash":
		return bash.Dialect(), wild.BashScope, "bash", nil
	case "zsh":
		return zsh.Dialect(), wild.ZshScope, "zsh", nil
	default:
		return syntax.Dialect{}, nil, "", fmt.Errorf("unknown dialect %q: bash or zsh", name)
	}
}

// isSet reports whether a flag was given on the command line, which is how an
// empty -reference is told from an absent one: empty means "trust every
// shebang" and absent means "the dialect's own shell".
func isSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func refName(s string) string {
	if s == "" {
		return "nothing"
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
