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
	"strings"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/wild"
)

func main() {
	var (
		dirs = flag.String("dirs", strings.Join(wild.DefaultDirs, ","),
			"comma-separated directories to sweep")
		reference = flag.String("reference", "bash",
			"shell that decides whether a refused file is really a shell script; empty to trust every shebang")
		verbose = flag.Bool("v", false, "list every failure's message in full")
		run     = flag.String("run", "",
			"also RUN each script that parses, under this binary and the reference, and report where they disagree")
		timeout = flag.Duration("timeout", 10*time.Second, "how long one run may take")
	)
	flag.Parse()
	// Resolved to a path before it is used, so a diagnostic naming the shell
	// can be recognized and so normalise is never handed a bare word. A name
	// that is not on PATH is left as it was and fails where it is used, which
	// says more than a failure here would.
	if p, err := exec.LookPath(*reference); err == nil {
		*reference = p
	}

	rep := wild.Sweep(context.Background(), strings.Split(*dirs, ","), bash.Dialect(), *reference)

	fmt.Printf("scripts found: %d   parsed: %d   refused by %s too: %d   failures: %d\n",
		rep.Scanned, rep.Parsed, refName(*reference), rep.NotShell, len(rep.Failures))
	if len(rep.Failures) > 0 {
		fmt.Println("\nnot parsed:")
		for _, f := range rep.Failures {
			msg := f.Err.Error()
			if !*verbose {
				msg = firstLine(msg)
			}
			fmt.Printf("  %s\n    %s\n", f.Path, msg)
		}
	}
	if *run != "" {
		runSweep(*run, *reference, strings.Split(*dirs, ","), rep, *timeout, *verbose)
	}
	// A report rather than a gate: the count is the point, and failing here
	// would fail on whichever machine happens to have the oddest scripts.
	_ = os.Stdout.Sync()
}

// runSweep executes the scripts that parse and compares the two shells.
//
// Only the ones that parse: a script this shell cannot read has already been
// reported, and running it would say the same thing twice.
func runSweep(ours, reference string, dirs []string, parsed wild.Report, timeout time.Duration, verbose bool) {
	failed := map[string]bool{}
	for _, f := range parsed.Failures {
		failed[f.Path] = true
	}
	var paths []string
	for _, p := range wild.Find(dirs) {
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
