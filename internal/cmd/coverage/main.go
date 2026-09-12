// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command coverage reports, per dialect, every element of the shell's surface
// that no case in the tree mentions.
//
// Report only, and never a gate — see internal/coverage for what a mention is
// and, more to the point, what it is not.
package main

import (
	"flag"
	"fmt"
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
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

type preset struct {
	name    string
	dialect func() syntax.Dialect
	apply   func(*interp.Runner)
}

func main() {
	list := flag.Int("list", 40, "how many never-mentioned names to print per kind; 0 for all")
	suite := flag.String("suite", "", "a directory of shell files to read alongside the corpus")
	flag.Parse()

	presets := []preset{
		{"core", syntax.Core, nil},
		{"bash", bash.Dialect, bash.Apply},
		{"zsh", zsh.Dialect, zsh.Apply},
		{"ksh", ksh.Dialect, ksh.Apply},
		{"dash", dash.Dialect, dash.Apply},
		{"ash", ash.Dialect, ash.Apply},
	}

	srcs := corpusSources()
	if *suite != "" {
		more, err := suiteSources(*suite)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		srcs = append(srcs, more...)
	}

	var cols []coverage.Column
	for _, p := range presets {
		r := &interp.Runner{}
		if p.apply != nil {
			p.apply(r)
		}
		col, err := coverage.Run(p.name, p.dialect(), r.BuiltinNames(), srcs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cols = append(cols, col)
	}
	fmt.Print(coverage.Report(cols, *list))
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

// suiteSources reads a directory of shell files, for the suite of our own
// that a later change adds. A path is fine here and is not the rule the
// fetched suites live under: these are our files, Apache-2.0 and readable.
func suiteSources(dir string) ([]coverage.Source, error) {
	var out []coverage.Source
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".tests") {
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
