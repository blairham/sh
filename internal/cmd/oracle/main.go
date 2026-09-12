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
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"sort"
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
		gated  = flag.Bool("gated", false, "run the corpus twice through -bin, with and without a sandbox policy, and report what the policy changed")
		pol    = flag.String("policy", "", "the policy file -gated uses; empty generates one confining writes to the scratch directory")
	)
	flag.Parse()

	if *gated {
		if err := runGated(*bin, strings.Fields(*bargs), *pol, *vrb); err != nil {
			fmt.Fprintln(os.Stderr, "oracle:", err)
			os.Exit(exitFailure)
		}
		return
	}

	if *bin != "" {
		rep, err := oracle.RunConformance(context.Background(), *bin, *ref, strings.Fields(*bargs), oracle.Corpus)
		if err != nil {
			fmt.Fprintln(os.Stderr, "oracle:", err)
			os.Exit(exitFailure)
		}
		fmt.Print(rep.Summary(*vrb))
		return
	}

	// Whatever routes the panel opened are closed here, once, however this
	// command ends. A container column is the only one that holds anything,
	// and a stray container on a machine running a dozen sessions is exactly
	// the kind of leftover nobody can tell from live work.
	defer oracle.Shutdown()

	if err := run(*check, *golden, *doc); err != nil {
		fmt.Fprintln(os.Stderr, "oracle:", err)
		os.Exit(exitFailure)
	}
}

func run(check bool, goldenPath, docPath string) error {
	// Asked first, and before a shell is run, because it is the half that
	// gives the same answer on every machine: the corpus, the record and the
	// rendered document either agree with each other or they do not. The
	// panel comparison below cannot make that claim — see RecordCheck — so
	// the two are reported apart rather than as one verdict.
	stale := false
	if check {
		bad, err := checkRecord(goldenPath, docPath)
		if err != nil {
			return err
		}
		stale = bad
	}

	got, err := oracle.Execute(context.Background(), oracle.Corpus)
	if err != nil {
		return err
	}

	for _, s := range got.Shells {
		fmt.Printf("  %-12s %s\n", s.Name, s.Version)
	}
	for _, a := range got.Absent {
		// Not a failure. A narrower panel is a weaker claim, and the right
		// response is to say so rather than to pretend or to refuse to run.
		//
		// One line per column, with the reason, rather than a list of names
		// on the end of the panel table. A name in a list beside seven
		// version strings reads as a footnote; `ash  NOT RUN` in the column
		// the versions are in does not, and telling those two apart is the
		// whole of what an honest degradation is. The reason matters as much
		// as the name: "install it" and "start Docker" are different work.
		fmt.Printf("  %-12s NOT RUN — %s\n", a.Name, a.Reason)
	}
	fmt.Printf("  %d cases across %d shells\n\n", len(oracle.Corpus), len(got.Shells))

	if !check {
		// The record is read before it is written: a racing row's cells are
		// one sample of a coin flip, and rewriting them is the only way such
		// a row can move, so they are carried forward. Run.Record owns the
		// order the two artifacts have to be produced in.
		prev, err := oracle.Load(goldenPath)
		switch {
		case err == nil:
		case errors.Is(err, fs.ErrNotExist):
			// The first run on a machine with no record, which is how the
			// file is created. Nothing to carry forward.
			prev = nil
		default:
			return fmt.Errorf("reading %s to carry racing rows forward: %w", goldenPath, err)
		}
		// Pinned before anything is counted or confirmed: a racing row's
		// cells are carried forward here, so asking what changed before
		// this would report every coin that landed the other way.
		got.KeepRacingRows(prev, oracle.Corpus)
		held, dropped, err := got.Confirm(context.Background(), prev, oracle.Corpus, oracle.Execute)
		if err != nil {
			return err
		}
		if err := got.Record(prev, oracle.Corpus, docPath, goldenPath); err != nil {
			return err
		}
		fmt.Printf("wrote %s and %s\n\n%s", docPath, goldenPath, oracle.CellChangeReport(held, dropped))
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

	drifts, unrepeated, err := oracle.ConfirmDrift(context.Background(), got.Compare(want), oracle.Corpus, oracle.Execute)
	if err != nil {
		return err
	}
	if len(unrepeated) > 0 {
		// Said before the verdict rather than after it, because it changes
		// how the verdict should be read: these are the rows that would
		// have sent somebody to look at a shell that never moved.
		fmt.Fprintf(os.Stderr, "%d case(s) differed on the first run and not on the second, so they are\n"+
			"not reported as drift — a measurement that will not repeat is the machine\n"+
			"and not the shell:\n\n", len(unrepeated))
		for _, d := range unrepeated {
			fmt.Fprintln(os.Stderr, "  "+d.String())
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(drifts) == 0 {
		// Qualified by what did not run, and never the bare sentence when
		// something did not. `no drift` over a panel missing a column is the
		// exact misreading this repository already has a scar from — a gate
		// inert on one platform reads as a pass — and it was available here
		// long before ash: bash32 is absent on every Linux machine and the
		// verdict said "the panel behaves as recorded" anyway.
		if silent := notRun(got, want); len(silent) > 0 {
			fmt.Printf("no drift among the %d column(s) that ran — but the record has %d this\n"+
				"machine did not run, so nothing here says anything about %s:\n",
				len(got.Shells), len(silent), strings.Join(silent, ", "))
			for _, a := range got.Absent {
				fmt.Printf("  %-12s NOT RUN — %s\n", a.Name, a.Reason)
			}
		} else {
			fmt.Println("no drift: the panel behaves as recorded.")
		}
		if stale {
			// Said twice on purpose. The record check runs first because it
			// needs no shells, and the panel run that follows takes half a
			// minute and prints six version lines, so the reason this
			// command failed would otherwise have scrolled off the screen
			// before it finished.
			fmt.Fprintln(os.Stderr, "\nbut the committed files still disagree with each other: run `make oracle`.")
			os.Exit(exitDrift)
		}
		return nil
	}

	if stale {
		fmt.Fprint(os.Stderr, "and the committed files disagree with each other, as above.\n\n")
	}
	fmt.Fprintf(os.Stderr, "%d case(s) drifted:\n\n", len(drifts))
	for _, d := range drifts {
		fmt.Fprintln(os.Stderr, "  "+d.String())
	}
	fmt.Fprintln(os.Stderr, "\n  by shell: "+byShell(drifts))
	fmt.Fprint(os.Stderr, `
Drift is not automatically a bug. A shell was upgraded, or a case was
edited, and the recorded behavior is no longer what the panel does.
Decide which, then update docs/spec to match and run `+"`make oracle`"+`.
The spec entries that cite these cases are now the ones to re-read.

This half of the check cannot be the same on two machines, which is why it
is reported rather than enforced in continuous integration: a runner does
not have the same builds of the same shells. Locally, where the panel is
the one that produced the record, it is the gate.
`)
	os.Exit(exitDrift)
	return nil
}

// notRun is the columns the golden record has and this run does not.
//
// It is asked of the *record* rather than of the panel because that is the
// question a reader of the verdict is really asking: the record makes a claim
// about ash, and this machine either re-checked it or did not.
func notRun(got, want *oracle.Run) []string {
	ran := map[string]bool{}
	for _, s := range got.Shells {
		ran[s.Name] = true
	}
	var silent []string
	for _, s := range want.Shells {
		if !ran[s.Name] {
			silent = append(silent, s.Name)
		}
	}
	return silent
}

// checkRecord compares the committed artifacts against each other and reports
// whether they disagree. It is deliberately separate from the panel run: this
// question needs no shells and has one right answer everywhere.
func checkRecord(goldenPath, docPath string) (bool, error) {
	golden, err := oracle.Load(goldenPath)
	if err != nil {
		return false, fmt.Errorf("%w (run `make oracle` to create it)", err)
	}
	doc, err := os.ReadFile(docPath)
	if err != nil {
		return false, err
	}
	rc := oracle.CheckRecord(golden, string(doc), oracle.Corpus)
	if rc.OK() {
		return false, nil
	}
	fmt.Fprint(os.Stderr, rc.String())
	fmt.Fprintln(os.Stderr)
	return true, nil
}

// byShell tallies a drift report by column.
//
// Four hundred lines of drift is not a report anyone reads, and the shape of
// it is the part worth seeing at a glance: drift concentrated in one column
// is a shell that moved, drift spread evenly is a record that did.
func byShell(drifts []oracle.Drift) string {
	n := map[string]int{}
	for _, d := range drifts {
		n[d.Shell]++
	}
	names := make([]string, 0, len(n))
	for name := range n {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if n[names[i]] != n[names[j]] {
			return n[names[i]] > n[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s %d", name, n[name]))
	}
	return strings.Join(parts, ", ")
}

// runGated is the sandbox column: the same corpus, run under a policy, with
// the difference from the unpolicied run reported.
//
// It is not a gate and never exits nonzero for a case that changed. A denial
// is the policy working, and a harness that failed on one would be a harness
// that made the boundary unwatchable — the number is the artifact, and
// somebody reading it is what turns a change in it into work. `make
// conformance` is a report for the same reason and says so.
func runGated(bin string, args []string, policy string, verbose bool) error {
	cleanup := func() {}
	if policy == "" {
		var err error
		policy, cleanup, err = oracle.WriteContainmentPolicy()
		if err != nil {
			return fmt.Errorf("writing the corpus policy: %w", err)
		}
	}
	defer cleanup()

	rep, err := oracle.RunGated(context.Background(), bin, args, policy, oracle.Corpus)
	if err != nil {
		return err
	}
	fmt.Print(rep.Summary(verbose))
	return nil
}
