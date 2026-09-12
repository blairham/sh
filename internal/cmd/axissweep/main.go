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
//
// It is **not** in `make check` and must not be: it is thousands of shell
// processes, and this repository already deleted a gate for costing every
// commit too much. It runs on demand, and its output is a backlog.
//
// The exit status is 1 when anything came out unpinned, because "fail if
// nothing fails" is the whole instrument. A clean struct exits 0. Under
// -presets the same rule applies to the thing that half finds: an entry no
// field comment has answered yet (#2060).
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
	flag.Parse()

	uses, err := axissweep.PresetUse()
	if err != nil {
		fmt.Fprintln(os.Stderr, "axissweep:", err)
		os.Exit(2)
	}
	if *presets {
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
	if len(res.Unpinned()) > 0 {
		os.Exit(1)
	}
}
