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
	"strings"

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
	)
	flag.Parse()

	rep := wild.Sweep(context.Background(), strings.Split(*dirs, ","), bash.Dialect(), *reference)

	fmt.Printf("scripts found: %d   parsed: %d   refused by %s too: %d   failures: %d\n",
		rep.Scanned, rep.Parsed, refName(*reference), rep.NotShell, len(rep.Failures))
	if len(rep.Failures) == 0 {
		return
	}
	fmt.Println("\nnot parsed:")
	for _, f := range rep.Failures {
		msg := f.Err.Error()
		if !*verbose {
			msg = firstLine(msg)
		}
		fmt.Printf("  %s\n    %s\n", f.Path, msg)
	}
	// A report rather than a gate: the count is the point, and failing here
	// would fail on whichever machine happens to have the oddest scripts.
	_ = os.Stdout.Sync()
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
