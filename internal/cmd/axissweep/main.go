// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command axissweep moves every axis in interp.Semantics and reports the ones
// nothing objected to.
//
// An axis records a measured disagreement between real shells. One that can
// be flipped with nothing failing is therefore not an untested code path — it
// is a fact nobody checked, which the next reader takes for one that was
// measured. See internal/axissweep for why the enumeration is derived from
// the struct rather than kept by hand.
//
//	make axis-sweep                          # the whole struct, every dialect
//	make axis-sweep ARGS='-only Replacement' # one axis while triaging it
//	make axis-sweep ARGS='-dialects zsh'     # one column
//	make axis-sweep ARGS='-json out.json'    # the flips, for a later pass
//	make axis-sweep ARGS=-presets            # the two lists, no shell run
//	make axis-coverage                       # what no dialect answers, no shell run
//
// It is **not** in `make check` and must not be: it is thousands of shell
// processes, and this repository already deleted a gate for costing every
// commit too much. It runs on demand, and its output is a backlog.
//
// `-coverage` is the exception and is in `make check`, as a test rather than
// as this command — it runs no shells at all. It asks the other question
// (#2340): not whether anything objects when an axis moves, but whether each
// dialect answers the axis in the first place. A `Semantics` axis with no
// value in a dialect refuses at run time in the shipped binary while `go test
// ./...` stays green, which is how #2272 reached a release.
//
// The exit status is 1 while anything is **untriaged** — a backlog entry no
// field comment has answered — because "fail if nothing fails" is the whole
// instrument and some entries can never be pinned at all. A struct whose
// every entry carries either a row that catches it or a recorded reason it
// cannot be caught exits 0 (#2057, #2060).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blairham/sh/internal/axissweep"
	"github.com/blairham/sh/internal/oracle"
)

func main() {
	bin := flag.String("bin", "build/axis-sh", "the shell built with -tags shaxissweep")
	golden := flag.String("golden", "internal/oracle/testdata/golden.json", "path to the golden record")
	only := flag.String("only", "", "sweep only the axes whose path contains this")
	dialects := flag.String("dialects", "", "comma-separated dialects to sweep (default all four)")
	jobs := flag.Int("jobs", 4, "how many corpus rows to run at once")
	out := flag.String("json", "", "write every flip to this file as JSON")
	unspecified := flag.Bool("unspecified", false, "also flip to and from the unspecified constant, which measures reachability rather than disagreement")
	quiet := flag.Bool("quiet", false, "only print the report")
	presets := flag.Bool("presets", false, "only ask the presets what they hold — no shell is run, which takes a second rather than an hour")
	coverage := flag.Bool("coverage", false, "only report the axes each dialect does not answer — no shell is run")
	write := flag.Bool("write", false, "with -coverage, rewrite the committed ledger of what is unanswered")
	flag.Parse()

	if *coverage {
		cov, err := axissweep.Coverage()
		if err != nil {
			fmt.Fprintln(os.Stderr, "axissweep:", err)
			os.Exit(2)
		}
		if *write {
			path, err := cov.WriteLedger()
			if err != nil {
				fmt.Fprintln(os.Stderr, "axissweep:", err)
				os.Exit(2)
			}
			fmt.Fprintf(os.Stderr, "wrote %d unanswered pairs to %s\n", len(cov.Gaps), path)
			// Deliberately still nonzero when the run had findings. A
			// regeneration is how a real gap gets recorded *and* how one
			// gets buried, and the difference is whether somebody read the
			// list — so the exit status keeps saying there was one.
		}
		fmt.Print(cov.Report())
		if cov.Failures() > 0 {
			os.Exit(1)
		}
		return
	}

	uses, err := axissweep.PresetUse()
	if err != nil {
		fmt.Fprintln(os.Stderr, "axissweep:", err)
		os.Exit(2)
	}
	if *presets {
		if cov, err := axissweep.Coverage(); err == nil {
			fmt.Print(cov.Report())
		}
		fmt.Print(axissweep.PresetReport(uses))
		// Same rule as the corpus half: the instrument exits nonzero when
		// it has found something, and what it finds here is an entry no
		// field comment has answered. A struct whose every preset-list
		// entry carries its verdict exits 0.
		if len(axissweep.Untriaged(uses)) > 0 {
			os.Exit(1)
		}
		return
	}

	record, err := oracle.Load(*golden)
	if err != nil {
		fmt.Fprintln(os.Stderr, "axissweep:", err)
		os.Exit(2)
	}

	opts := axissweep.Options{
		Bin:         *bin,
		Cases:       oracle.Corpus,
		Golden:      record,
		Jobs:        *jobs,
		Only:        *only,
		Unspecified: *unspecified,
	}
	if !*quiet {
		opts.Log = os.Stderr
	}
	if *dialects != "" {
		var kept []axissweep.Target
		for _, t := range axissweep.Targets() {
			for _, name := range strings.Split(*dialects, ",") {
				if strings.TrimSpace(name) == t.Dialect {
					kept = append(kept, t)
				}
			}
		}
		if len(kept) == 0 {
			fmt.Fprintf(os.Stderr, "axissweep: no dialect named in %q\n", *dialects)
			os.Exit(2)
		}
		opts.Targets = kept
	}

	res, err := axissweep.Run(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "axissweep:", err)
		os.Exit(2)
	}
	if *out != "" {
		blob, err := json.MarshalIndent(res, "", "  ")
		if err == nil {
			err = os.WriteFile(*out, append(blob, '\n'), 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "axissweep:", err)
			os.Exit(2)
		}
	}
	res.Presets = uses
	fmt.Print(res.Report())
	fmt.Print(axissweep.PresetReport(uses))
	// Nonzero while anything is *untriaged* rather than while anything is
	// unpinned. Some pairs are permanent — an axis a dialect never consults
	// cannot be pinned by any row — so "unpinned reaches zero" was never a
	// state this could be in, and an exit status nobody can clear is one
	// nobody reads. See #2057.
	if res.Untriaged()+len(axissweep.Untriaged(uses)) > 0 {
		os.Exit(1)
	}
}
