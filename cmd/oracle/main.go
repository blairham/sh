// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command oracle records what real shells do, and fails when that changes.
//
// It implements no shell behavior; it runs shell binaries over the corpus in
// internal/oracle and writes down what happened. That is what makes the specs
// in docs/spec re-runnable rather than merely asserted.
//
//	oracle              # regenerate the measurements and the golden record
//	oracle -check       # fail if the panel's behavior moved
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blairham/sh/internal/oracle"
)

const (
	exitOK      = 0
	exitDrift   = 1
	exitFailure = 2
)

func main() {
	var (
		check  = flag.Bool("check", false, "compare against the golden record instead of rewriting it")
		golden = flag.String("golden", "internal/oracle/testdata/golden.json", "path to the golden record")
		doc    = flag.String("doc", "docs/spec/measurements.md", "path to the generated measurements")
		bin    = flag.String("bin", "", "grade this binary against the panel instead of recording it")
		ref    = flag.String("against", "bash", "which panel shell to grade against")
		vrb    = flag.Bool("v", false, "list the cases that do not match")
		bargs  = flag.String("binargs", "", "space-separated flags the binary needs, before -c")
	)
	flag.Parse()

	if *bin != "" {
		rep, err := oracle.RunConformance(context.Background(), *bin, *ref, strings.Fields(*bargs), oracle.Corpus)
		if err != nil {
			fmt.Fprintln(os.Stderr, "oracle:", err)
			os.Exit(exitFailure)
		}
		fmt.Print(rep.Summary(*vrb))
		return
	}

	if err := run(*check, *golden, *doc); err != nil {
		fmt.Fprintln(os.Stderr, "oracle:", err)
		os.Exit(exitFailure)
	}
}

func run(check bool, goldenPath, docPath string) error {
	got, err := oracle.Execute(context.Background(), oracle.Corpus)
	if err != nil {
		return err
	}

	for _, s := range got.Shells {
		fmt.Printf("  %-12s %s\n", s.Name, s.Version)
	}
	if len(got.Missing) > 0 {
		// Not a failure. A narrower panel is a weaker claim, and the right
		// response is to say so rather than to pretend or to refuse to run.
		fmt.Printf("  not present: %v\n", got.Missing)
	}
	fmt.Printf("  %d cases across %d shells\n\n", len(oracle.Corpus), len(got.Shells))

	if !check {
		if err := os.WriteFile(docPath, []byte(got.Markdown(oracle.Corpus)), 0o644); err != nil {
			return err
		}
		if err := got.Save(goldenPath); err != nil {
			return err
		}
		fmt.Printf("wrote %s and %s\n", docPath, goldenPath)
		return nil
	}

	want, err := oracle.Load(goldenPath)
	if err != nil {
		return fmt.Errorf("%w (run `make oracle` to create it)", err)
	}

	if newCases := got.NewCases(want); len(newCases) > 0 {
		fmt.Fprintf(os.Stderr, "%d case(s) not in the golden record:\n", len(newCases))
		for _, id := range newCases {
			fmt.Fprintln(os.Stderr, "  "+id)
		}
		fmt.Fprintln(os.Stderr, "\nRun `make oracle` to record them.")
		os.Exit(exitDrift)
	}

	drifts := got.Compare(want)
	if len(drifts) == 0 {
		fmt.Println("no drift: the panel behaves as recorded.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "%d case(s) drifted:\n\n", len(drifts))
	for _, d := range drifts {
		fmt.Fprintln(os.Stderr, "  "+d.String())
	}
	fmt.Fprint(os.Stderr, `
Drift is not automatically a bug. A shell was upgraded, or a case was
edited, and the recorded behavior is no longer what the panel does.
Decide which, then update docs/spec to match and run `+"`make oracle`"+`.
The spec entries that cite these cases are now the ones to re-read.
`)
	os.Exit(exitDrift)
	return nil
}
