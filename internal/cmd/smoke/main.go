// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command smoke drives a realistic interactive session through a real
// terminal, for each dialect binary it is given, and prints what worked.
//
// It answers one question — is this shell usable yet — and it answers it as a
// table rather than as a status, because "no" is not a useful answer and "no,
// and here is which of the fourteen things a person does at a prompt are
// missing" is.
//
// Run it with `make smoke`. It is not a gate: it runs real binaries on a real
// pseudo-terminal with real external commands and asserts on a job resuming,
// none of which is deterministic the way `make check` is. The exit status is
// still worth having — nonzero when something failed that no issue is known to
// own — so that it can become one later without being rewritten.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

func main() {
	var (
		bashBin = flag.String("bash", "", "the bash dialect binary to drive")
		zshBin  = flag.String("zsh", "", "the zsh dialect binary to drive")
		root    = flag.String("home", "",
			"where to make the scratch homes; a temporary directory that is removed afterwards by default")
		verbose = flag.Bool("v", false, "print the detail for every row, not only for the ones that did not pass")
		list    = flag.Bool("known", false, "print the rows known to be missing, and the issue that owns each")
	)
	flag.Parse()

	if *list {
		fmt.Print(smoke.KnownList())
		return
	}

	type job struct {
		dialect smoke.Dialect
		bin     string
	}
	var jobs []job
	if *bashBin != "" {
		jobs = append(jobs, job{smoke.Bash(), *bashBin})
	}
	if *zshBin != "" {
		jobs = append(jobs, job{smoke.Zsh(), *zshBin})
	}
	if len(jobs) == 0 {
		fmt.Fprintln(os.Stderr, "smoke: name at least one binary with -bash or -zsh")
		os.Exit(2)
	}

	ctx := context.Background()
	started := time.Now()
	reports := make([]smoke.Report, 0, len(jobs))
	for _, j := range jobs {
		reports = append(reports, smoke.Run(ctx, j.dialect, smoke.Config{Bin: j.bin, Root: *root}))
	}

	text, status := smoke.Render(reports, time.Since(started), *verbose)
	fmt.Print(text)
	os.Exit(status)
}
