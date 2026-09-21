// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command coverage reports, per dialect, every element of the shell's surface
// that no case in the tree mentions.
//
// Report only, and never a gate — see internal/coverage for what a mention is
// and, more to the point, what it is not.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/coverage"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/internal/suite"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

type preset struct {
	name    string
	dialect func() syntax.Dialect
	apply   func(*interp.Runner)
}

// DefaultSuite is our own committed suite, and it is read on every run.
//
// It was a flag defaulting to the empty string for as long as there was
// nothing to point it at. There is now: 35 files across core/, ext/ and the
// five dialect tiers. Nothing passed the flag, so the work-list this
// instrument prints was computed from the corpus alone and named `-le`, `-ne`
// and `ParamLowerFirst` as elements nobody had asked about — all three asked
// by our own suite, for a day, in files written by this campaign (#2630).
//
// A default rather than a line in the Makefile, so a bare `go run
// ./internal/cmd/coverage` is the same measurement the target reports. Two
// spellings of one instrument is how the two drift apart.
const DefaultSuite = suite.OurRoot

func main() {
	list := flag.Int("list", 40, "how many never-mentioned names to print per kind; 0 for all")
	suiteDir := flag.String("suite", DefaultSuite,
		"a directory of our own .tests files to read alongside the corpus; empty reads the corpus alone")
	flag.Parse()

	srcs := corpusSources()
	read := []coverage.Origin{{Name: "corpus cases", Count: len(srcs)}}
	if *suiteDir != "" {
		more, err := suiteSources(*suiteDir)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !given("suite"):
			// The default, from a directory this run cannot see — a build
			// from somewhere other than the module root. Not fatal, because
			// the corpus half of the report is still a true thing about the
			// corpus, and not silent either: understating the denominator
			// without saying so is the whole of #2630.
			read = append(read, coverage.Origin{
				Name: *suiteDir + " — not found from here, so this is the corpus alone",
			})
		case err != nil:
			// Asked for by name and not there. That is a typo or a moved
			// directory, and carrying on would answer a question nobody put.
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		default:
			srcs = append(srcs, more...)
			read = append(read, coverage.Origin{
				Name:  "files under " + *suiteDir,
				Count: len(more),
			})
		}
	}

	cols, err := columns(presets(), srcs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(coverage.Report(cols, *list, read))
}

// presets is the six columns this command reports, and it is a function so
// that the guard in main_test.go grades the same six. Two spellings of the
// column list is how the report and the thing that gates it come apart.
func presets() []preset {
	return []preset{
		{"core", syntax.Core, nil},
		{"bash", bash.Dialect, bash.Apply},
		{"zsh", zsh.Dialect, zsh.Apply},
		{"ksh", ksh.Dialect, ksh.Apply},
		{"dash", dash.Dialect, dash.Apply},
		{"ash", ash.Dialect, ash.Apply},
	}
}

// columns runs every preset over the same sources.
func columns(ps []preset, srcs []coverage.Source) ([]coverage.Column, error) {
	var cols []coverage.Column
	for _, p := range ps {
		r := &interp.Runner{}
		if p.apply != nil {
			p.apply(r)
		}
		col, err := coverage.Run(p.name, p.dialect(), r.BuiltinNames(), srcs)
		if err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// corpusSources is every case the oracle records, which is the body of cases
// this tree actually has today.
func corpusSources() []coverage.Source {
	out := make([]coverage.Source, 0, len(oracle.Corpus))
	for _, c := range oracle.Corpus {
		out = append(out, coverage.Source{Label: c.ID, Text: c.Snippet})
	}
	return out
}

// given reports whether a flag was typed rather than left at its default.
//
// The difference decides what a missing suite directory means: named on the
// command line it is a mistake worth stopping for, left at the default it is
// a run from somewhere other than the module root.
func given(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// suiteSources reads our own suite. A path is fine here and is not the rule
// the fetched suites live under: these are our files, Apache-2.0 and
// readable.
func suiteSources(dir string) ([]coverage.Source, error) {
	var out []coverage.Source
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, suite.OurExt) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, coverage.Source{Label: path, Text: string(b)})
		return nil
	})
	return out, err
}
