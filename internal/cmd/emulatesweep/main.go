// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command emulatesweep reports how far `emulate MODE` reaches into the
// grammar, by asking every corpus snippet whether the three modes disagree
// about it.
//
//	make emulate-sweep                      # the whole corpus, both binaries
//	make emulate-sweep ARGS='-only pat/'    # one family, while working a row
//	make emulate-sweep ARGS=-rows           # every disagreeing case, listed
//
// It is the bar for the `emulate MODE reaches the grammar` epic and the check
// for every row under it. The number is the count of snippets the reference
// and this shell answer differently about, and it is 155 today.
//
// It is **not** in `make check` and must not be: it is some twenty-seven
// thousand shell processes. It runs on demand, and its output is a backlog.
//
// The exit status is 1 while anything disagrees, which is the expected state
// until the epic closes, and 2 when the instrument could not be trusted —
// a control that did not fire, a selection that matched nothing, a shell that
// hung. Those two are deliberately different: a sweep that reports zero
// because it was broken must not look like a sweep that reports zero because
// the work is done.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/blairham/sh/internal/emulatesweep"
	"github.com/blairham/sh/internal/oracle"
)

func main() {
	bin := flag.String("bin", "build/emulate-zsh", "our zsh binary, the one being graded")
	ref := flag.String("ref", "", "the reference zsh (default: the oracle panel's)")
	only := flag.String("only", "", "sweep only the cases whose ID contains this")
	jobs := flag.Int("jobs", 8, "how many snippets to ask at once")
	rows := flag.Bool("rows", false, "also list every disagreeing case")
	flag.Parse()

	reference, err := resolveReference(*ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, "emulatesweep:", err)
		os.Exit(2)
	}

	ours := emulatesweep.Binary{
		Name:    "ours",
		Path:    *bin,
		Version: oracle.Version(context.Background(), *bin),
	}

	res, err := emulatesweep.Sweep(context.Background(), emulatesweep.Options{
		Reference: reference,
		Ours:      ours,
		Cases:     oracle.Corpus,
		Only:      *only,
		Jobs:      *jobs,
	})
	if err != nil {
		// A control that did not fire, or a selection holding nothing. Either
		// way there is no number, and printing one anyway is the whole
		// failure this instrument is written against.
		fmt.Fprintln(os.Stderr, "emulatesweep:", err)
		os.Exit(2)
	}

	fmt.Print(res.Report())
	if *rows {
		fmt.Print("\n" + res.Rows())
	}
	if len(res.Disagreements) > 0 {
		os.Exit(1)
	}
}

func resolveReference(path string) (emulatesweep.Binary, error) {
	if path == "" {
		return emulatesweep.ResolveReference()
	}
	return emulatesweep.Binary{
		Name:    "reference",
		Path:    path,
		Version: oracle.Version(context.Background(), path),
	}, nil
}
