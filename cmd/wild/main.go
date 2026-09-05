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
			"comma-separated directories to sweep")
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

	scope := wild.Scope{Dirs: strings.Split(*dirs, ","), Shells: shells, Depth: *depth}
	if *list {
		// The run sweep executes third-party code, and the only responsible
		// way to choose what it runs is to read the scripts first. Nothing
		// else prints the population, so choosing a subset meant guessing at
		// it — which is how a sweep ends up running something nobody looked at.
		paths, _ := wild.Find(scope)
		for _, p := range paths {
			fmt.Println(p)
		}
		return
	}
	rep := wild.Sweep(context.Background(), scope, dialect, *reference)

	fmt.Printf("scripts found: %d   parsed: %d   refused by %s too: %d   failures: %d\n",
		rep.Scanned, rep.Parsed, refName(*reference), rep.NotShell, len(rep.Failures))
	// Skipped files are counted and never named: printing a path would
	// invite the reader to open a file CLEANROOM.md forbids opening.
	if len(rep.Skipped) > 0 {
		fmt.Printf("skipped without reading: %s — CLEANROOM.md forbids opening these, so they are counted, never listed\n",
			skipSummary(rep.Skipped))
	}
	report(rep, *verbose)
	if *run != "" {
		runSweep(*run, *reference, scope, rep, *timeout, *verbose)
	}
	// A report rather than a gate: the count is the point, and failing here
	// would fail on whichever machine happens to have the oddest scripts.
	_ = os.Stdout.Sync()
}

// report prints the failures ranked by cause, largest first.
//
// Ranked rather than listed, because the list is the wrong artifact: a reader
// facing sixty lines cannot tell whether that is sixty problems or three, and
// the sweep is the only thing that can tell them. The paths stay available
// under -v, since a cause without an example is a category rather than a bug.
func report(rep wild.Report, verbose bool) {
	causes := wild.Causes(rep.Failures)
	if len(causes) == 0 {
		return
	}
	fmt.Printf("\nnot parsed, by cause (%d causes over %d scripts):\n", len(causes), len(rep.Failures))
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
func runSweep(ours, reference string, scope wild.Scope, parsed wild.Report, timeout time.Duration, verbose bool) {
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
	fmt.Printf("\nran: %d   agreed: %d   timed out: %d   not the same twice: %d   disagreed: %d\n",
		rep.Ran, rep.Agreed, rep.Timedout, rep.Unstable, len(rep.Mismatches))
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
