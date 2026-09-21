// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command suitecheck fetches a shell's own test suite and runs every file of
// it through both that shell and the dialect binary claiming to be it.
//
// It is a report and never a gate, for the reason `make wild` is not one and
// one more: the corpus is fetched over a network from a third party, so a
// machine with no network has not told us anything about the shell. Every
// reason it cannot run is printed in full and exits 0 — an instrument that
// failed a build because a download did not arrive would be turned off within
// a week, and an instrument that exited quietly would be worse, because zero
// files compared and zero disagreements look identical in a log.
//
// What it prints is counts and this parser's own diagnostics. It never prints
// a line, a token or a path from a fetched file: those are another project's
// expression, CLEANROOM.md's red list covers them, and the carve-out this
// stands on is a carve-out for running a suite rather than for reading one.
//
// # -own: our own suite, the same grader
//
// `make suite` runs this with -own, over the files committed under
// share/suite. It is the same instrument and deliberately so: a column there
// is a suite.Suite with Ours set and its files on disk instead of in an
// archive, and it is graded by the same suite.Sweep, against the same
// reference shell, with the same three numbers. Nothing in -own mode computes
// a score of its own. A second scorer that drifted from the first is a
// failure this repository has made before.
//
// Two things do differ, and both follow from whose files they are. Ours are
// committed and Apache-2.0, so the report names the cases that failed — a
// report that would not say which of our own cases to go and fix is not a
// work list. And core/ runs in every column, so -own additionally asks
// whether the reference shells themselves all wrote the same bytes: that is
// the common-denominator claim in docs/spec/shell-matrix.md turned into a
// measurement rather than an assertion.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/suite"
)

func main() {
	var (
		dialect = flag.String("dialect", "bash",
			"which column to run: the dialect binary's name under cmd/")
		bin = flag.String("bin", "",
			"the dialect binary to grade; required")
		buildDir = flag.String("build", "build",
			"directory the suite is fetched into; it is gitignored and nothing here is ever committed")
		timeout = flag.Duration("timeout", suite.DefaultTimeout,
			"how long one file gets under one shell before its process group is killed")
		jobs    = flag.Int("jobs", 4, "how many files to run at once")
		refetch = flag.Bool("refetch", false,
			"fetch the archive again even if the unpacked tree matches the pin")
		only = flag.String("only", "",
			"comma-separated file names to run, for working on the harness itself")
		panel = flag.Bool("panel", false, "print the panel of columns and stop")
		cells = flag.Bool("cells", false,
			"print #2291's leg-2 roll-up — column x area, with the cells closed by "+
				"measurement — and stop. Starts no shell.")
		own = flag.Bool("own", false,
			"grade our own committed suite instead of a fetched one, every column at once")
		ownColumn = flag.String("column", "",
			"with -own, run only these columns, comma-separated. A change to how one "+
				"column reaches its reference is then exercised without the whole sweep, "+
				"which is four dialect binaries over every file under two shells each. The "+
				"cross-check, the only-here checks and the cell roll-up are left out of a "+
				"scoped run: each of those is a claim over every column")
		ownRoot = flag.String("root", suite.OurRoot,
			"where our own suite lives in the tree")
		ownBins = binSet{}
	)
	flag.Var(ownBins, "own-bin",
		"in -own mode, a dialect binary to grade: dialect=path, repeated")
	flag.Parse()

	if *panel {
		printPanel()
		return
	}

	// Before the context, because this starts nothing: the roll-up is a
	// derivation over the table and the files, so it costs a directory read
	// and answers on a machine with no shells and no container runtime at
	// all. That is what makes it quotable from a pull request rather than
	// from whoever last ran the sweep.
	if *cells {
		if code := printCells(*ownRoot); code != 0 {
			os.Exit(code)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *own {
		if code := runOwn(ctx, *ownRoot, ownBins, *timeout, *jobs, *only, names(*ownColumn)); code != 0 {
			os.Exit(code)
		}
		return
	}

	if code := run(ctx, *dialect, *bin, *buildDir, *timeout, *jobs, *refetch, *only); code != 0 {
		os.Exit(code)
	}
}

func run(ctx context.Context, dialect, bin, buildDir string, timeout time.Duration, jobs int, refetch bool, only string) int {
	s, ok := suite.Find(dialect)
	if !ok {
		fmt.Fprintf(os.Stderr, "suitecheck: no column for dialect %q; -panel lists them\n", dialect)
		return 2
	}
	if s.NotYet != "" {
		// Loud, and zero. A column that is not built is a fact about this
		// instrument and printing it is the whole reason the unbuilt columns
		// are rows at all: a missing column that says nothing reads as a
		// column that passed.
		fmt.Printf("%s: not yet a column of this instrument.\n\n  %s\n\n", s.Name, wrap(s.NotYet, "  "))
		printPanel()
		return 0
	}
	if bin == "" {
		fmt.Fprintln(os.Stderr, "suitecheck: -bin is required: the dialect binary to grade")
		return 2
	}
	bin, err := suite.Shell(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suitecheck: -bin: %v\n", err)
		return 2
	}

	reference, found := suite.Locate(s.Lookup)
	if !found {
		skip(fmt.Sprintf("no %s on this machine to be the oracle", s.Name),
			"The reference shell is the oracle, never the suite's own expected output: "+
				"those files were generated by a different build of a different version. "+
				"Looked in "+strings.Join(s.Lookup, ", ")+".")
		return 0
	}

	dir, err := suite.Fetch(ctx, s, buildDir, refetch)
	switch {
	case errors.Is(err, suite.ErrOffline):
		skip("the suite could not be fetched",
			"It is never committed — it is "+licenseNote(s)+" and this repository is Apache-2.0, "+
				"so committing it would relicense the tree by accident. That means a machine "+
				"with no network cannot run this column.\n\n  "+err.Error())
		return 0
	case errors.Is(err, suite.ErrDigest):
		fmt.Fprintf(os.Stderr, "suitecheck: %v\n\n", err)
		fmt.Fprintln(os.Stderr, "  The archive is pinned by digest so that two runs a month apart are")
		fmt.Fprintln(os.Stderr, "  comparable. Grading whatever arrived would move every number in the")
		fmt.Fprintln(os.Stderr, "  report with nothing in the report saying so.")
		return 1
	case err != nil:
		fmt.Fprintf(os.Stderr, "suitecheck: %v\n", err)
		return 1
	}

	helpers, err := suite.BuildHelpers(ctx, s, dir)
	if errors.Is(err, suite.ErrNoCompiler) {
		skip("the suite's helpers cannot be built",
			"Without them the whole suite scores zero for reasons that have nothing to do "+
				"with us: the calls fail under both shells and the two failures match, so the "+
				"run reads as agreement on an error message.")
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "suitecheck: building the suite's helpers: %v\n", err)
		return 1
	}

	opts := suite.Options{Timeout: timeout, Jobs: jobs, Only: names(only)}
	rep, err := suite.Sweep(ctx, s, dir, bin, reference, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suitecheck: %v\n", err)
		return 1
	}
	rep.ReferenceVersion = s.Identify(ctx, reference)
	rep.Helpers = helpers
	printReport(os.Stdout, rep)
	printPanel()
	return 0
}

func names(list string) map[string]bool {
	if list == "" {
		return nil
	}
	set := map[string]bool{}
	for _, n := range strings.Split(list, ",") {
		if n = strings.TrimSpace(n); n != "" {
			set[n] = true
		}
	}
	return set
}

func licenseNote(s suite.Suite) string {
	if s.Name == "bash" {
		return "GPLv3"
	}
	return "another project's work"
}

// skip prints the reason a column did not run, in full, and returns nothing.
// The wording is deliberate: a skip is a fact about the machine, and the
// thing that must not happen is for it to be mistaken for a result.
func skip(what, why string) {
	fmt.Printf("SKIPPED — %s\n\n  %s\n\n", what, wrap(why, "  "))
}

// printReport writes the run, and every line of it has to be true without
// qualification: other sessions read these numbers to decide what to work on.
//
// It takes a writer so that the wording can be tested against the numbers it
// stands next to. That is not fastidiousness — the header carried a sentence
// for months saying a refused parse forfeited a whole file, which this
// instrument's own per-file results disprove on every run (#2381).
// out is the report's writer, and it drops the error every write returns.
//
// A report goes to a terminal or to a descriptor somebody already opened. A
// short write there is not something this program can do anything about, and
// checking one at every line would bury the wording — which is the part of
// this function that has to be right — under the handling.
type out struct{ w io.Writer }

func (o out) printf(format string, a ...any) { _, _ = fmt.Fprintf(o.w, format, a...) }
func (o out) println(a ...any)               { _, _ = fmt.Fprintln(o.w, a...) }

func printReport(w io.Writer, rep suite.Report) {
	o := out{w}
	s := rep.Suite
	o.printf("%s's own suite, %s — %d files (%s)\n", s.Name, s.Version, rep.Files, s.Ext)
	o.printf("  oracle   %s — %s\n", rep.Reference, rep.ReferenceVersion)
	if label, why := s.Lineage(rep.ReferenceVersion); why != "" {
		o.printf("  %s\n", wrap(label+" — "+why, "    "))
	}
	o.printf("  ours     %s\n", rep.Ours)
	if len(rep.Helpers) > 0 {
		o.printf("  helpers  %s, built from the suite's own C\n", strings.Join(rep.Helpers, ", "))
	}
	o.println()

	o.printf("  static read     %-9s %5.1f%%   the whole file parsed at once, in the dialect's\n",
		fmt.Sprintf("%d/%d", rep.Parsed, rep.Files), 100*rep.ParseRate())
	o.printf("                                     defaults — a read the shell itself never performs\n")
	o.printf("  strict          %-9s %5.1f%%   output and status identical, once each shell's own\n",
		fmt.Sprintf("%d/%d", rep.Strict, rep.Scored), 100*rep.StrictRate())
	o.printf("                                     path and the run's temp directory are taken out\n")
	o.printf("  line agreement  %-9s %5.1f%%   longest common subsequence of the two outputs'\n",
		"", 100*rep.LineRate())
	o.printf("                                     lines over the longer side, scored files only\n")
	o.printf("                  %-9s %5.1f%%   the same, unweighted per file\n",
		"", 100*rep.MeanFile)
	o.println()

	printDiffering(o, rep)

	printRefused(o, rep)

	o.printf("  not scored       %d unstable · %d oracle hung · %d dialect hung\n",
		rep.Unstable, rep.OracleHung, rep.DialectHung)
	if rep.OracleFailed+rep.DialectFailed > 0 {
		o.printf("                   %d oracle · %d dialect never started at all\n",
			rep.OracleFailed, rep.DialectFailed)
	}
	o.println("                   unstable: the oracle did not repeat itself, so the file is")
	o.println("                     evidence about neither shell — a pid, a clock, an order.")
	o.println("                   oracle hung: a harness fault. The shell that wrote the file")
	o.println("                     does not hang on it, so this is load or too tight a bound.")
	o.println("                   dialect hung: a hang we published. Different finding, kept apart.")
	if rep.LineCapped > 0 {
		o.printf("  capped           %d files printed more lines than the comparison's bound\n", rep.LineCapped)
	}
	o.println()

	o.println("  Each number misleads on its own. Strict reads as catastrophe — a suite file")
	o.println("  is hundreds of assertions and one disagreement forfeits all of them. Line")
	o.println("  agreement reads as triumph for the mirror-image reason, since most lines of")
	o.println("  most files are text a shell echoed back.")
	o.println()
	o.println("  The static read does not explain either of them, and this header used to say")
	o.println("  it did. This shell parses incrementally, exactly as the reference does: a")
	o.println("  file runs up to the construct that stopped the read, so a refusal costs the")
	o.println("  rest of one file rather than the file, and a refused file is run and scored")
	o.println("  above like any other. What the refusals cost is the band, measured. The")
	o.println("  static read is about the other route — `-n`, a formatter, an editor — which")
	o.println("  is a real property with real consumers in this tree and is simply not what")
	o.println("  the two runtime numbers are made of.")
	o.println()

	if len(rep.Causes) == 0 {
		o.println("  nothing was refused by the static read")
	} else {
		o.println("  constructs the static read refused, by files")
		o.println("  (this parser's own diagnostic, with the position and any word from the")
		o.println("   file removed — the fetched text that provoked it is never printed)")
		for _, c := range rep.Causes {
			o.printf("    %4d  %s", c.Files, c.Reason)
			if c.ReferenceRefuses > 0 {
				o.printf("   (%d of them the reference's own -n refuses too)", c.ReferenceRefuses)
			}
			o.println()
		}
	}
	o.println()

	printExcuses(o, rep)

	if len(rep.StatusPairs) > 0 {
		o.println("  exit status of the files that ran and disagreed  (ours / oracle)")
		o.println("  (numbers, because our runtime diagnostics would quote the file back;")
		o.println("   -1 is a run that ended on a signal or never started)")
		for _, p := range rep.StatusPairs {
			o.printf("    %4d  %d / %d\n", p.Files, p.Ours, p.Reference)
		}
		o.println()
	}
}

// printDiffering is the figure this column is burned down by, and the two
// things it is made of that the single number hides.
//
// It is the larger of the two sides per file — the reference's lines we never
// printed, and ours it never asked for — so a column can be carrying a large
// figure because it prints too much rather than because it answers too
// little, and those are different work. Both are printed.
//
// And some of the reference's side is not available to anybody here: the text
// of its own help builtin, the usage block it answers a bad option with, its
// version and its license. Matching those means copying them, which
// CLEANROOM.md's red list forbids, so they are counted and taken off rather
// than left in a number somebody estimates from. See [suite.Doc].
func printDiffering(o out, rep suite.Report) {
	differing := rep.Longest - rep.Common
	if differing == 0 {
		return
	}
	o.printf("  differing lines  %6d   the longer side of each file, less the lines the two\n", differing)
	o.printf("                            runs have in common — the figure a burndown moves\n")
	o.printf("                   %6d   the reference printed and we did not\n", rep.Missing)
	o.printf("                   %6d   we printed and the reference never asked for\n", rep.Excess)
	if !rep.ProseAsked() {
		o.println()
		return
	}
	o.printf("                   %6d   of the first, the reference quoting its own\n", rep.Prose)
	o.printf("                            documentation: help text, a usage block, a version\n")
	o.printf("                            and a license. Not work — matching it means copying\n")
	o.printf("                            it. A floor, counted by asking the shell for its own\n")
	o.printf("                            help and testing membership, never by reading\n")
	printReordered(o, rep)
	o.println()
}

// printRefused is what a refused static read actually cost, and what part of
// it no parser change can recover.
//
// It exists because the sentence it replaced was wrong in both halves. The
// refused files are not withheld from the run and not withheld from the
// score; and two constructs wearing this one row are different findings —
// one this parser cannot read, which is a defect, and one no static read can
// reach, which is not. The reference shell's own `-n` tells them apart.
func printRefused(o out, rep suite.Report) {
	if rep.Refused.Files == 0 {
		return
	}
	o.printf("  of the %d files the static read refused\n", rep.Refused.Files)
	o.printf("    %4d  ran and were scored anyway, at %d/%d strict and %.1f%% line agreement:\n",
		rep.Refused.Scored, rep.Refused.Strict, rep.Refused.Scored, 100*rep.Refused.LineRate())
	o.println("          a refused read forfeits no evidence, because the shell never takes")
	o.println("          that route — it parses incrementally, as the reference does")
	o.printf("    %4d  the reference's own -n refuses too, so no static read of the file\n",
		rep.ReferenceRefuses)
	o.println("          succeeds and this is not a gap in this parser: an option set at run")
	o.println("          time decides what a later line means, and a static read has no run time")
	o.println()
}

// printExcuses is the runtime half of a disagreement, ranked — and it is the
// half this report used to have nothing to say about.
//
// The line below the status table has been true for as long as it has been
// there: a whole diagnostic of ours names the command, the word or the option
// that provoked it, and those come from a file nobody here may read. Our own
// *catalog* carries none of that, so counting how many unmatched lines
// carry each phrase says what this shell refused without printing anything of
// the file — the same standard the static read's causes already meet.
func printExcuses(o out, rep suite.Report) {
	if len(rep.Excuses) == 0 {
		return
	}
	o.println("  what we said and the reference did not, by lines")
	o.println("  (our own catalog only — a whole diagnostic names the word that provoked")
	o.println("   it and that word is the file's; these phrases are ours)")
	for _, e := range rep.Excuses {
		o.printf("    %4d  %s\n", e.Lines, e.Phrase)
	}
	o.println()
}

// printPanel prints every column, built and unbuilt, every time.
//
// Always, and not only when one is asked for. The multi-dialect shape is the
// point of this instrument — a bash-only one would bend the substrate toward
// bash, which is what this project's premise forbids — and a column that is
// not built has to be visible as a column rather than as a silence.
func printPanel() {
	fmt.Println("  columns")
	for _, s := range suite.Panel {
		state := "ready"
		if s.NotYet != "" {
			state = "not yet"
		}
		version := s.Version
		if version == "" {
			version = "—"
		}
		fmt.Printf("    %-7s %-8s %-8s make %s-suite\n", s.Name, version, state, s.Dialect)
		if s.NotYet != "" {
			fmt.Printf("      %s\n", wrap(s.NotYet, "      "))
		}
	}
	fmt.Println()
	fmt.Println("  Nothing fetched here is ever committed. It lands under the gitignored build")
	fmt.Println("  directory, in a tree named for the distribution so that `make wild` refuses")
	fmt.Println("  to open it, and only the suite is unpacked — the shell's own source streams")
	fmt.Println("  past without reaching the disk, and neither does the suite's expected output.")
}

// wrap folds a reason to a readable width at the given indent.
func wrap(text, indent string) string {
	const width = 74
	var out strings.Builder
	line := 0
	for i, word := range strings.Fields(text) {
		if line > 0 && line+1+len(word) > width {
			out.WriteString("\n" + indent)
			line = 0
		} else if i > 0 {
			out.WriteString(" ")
			line++
		}
		out.WriteString(word)
		line += len(word)
	}
	return out.String()
}

// printReordered is the second floor under a differing-line count, and the
// one that comes from an order rather than from a text.
//
// An associative array has no order a script can ask for, and the reference
// lists its keys in its own hash table's order. Reproducing that means
// reproducing the hash function, the table size and the growth policy, none
// of which is in any vendor manual and all of which is in the source
// CLEANROOM.md's red list covers — so it is a floor, and #3304 records the
// cost rather than a way to close it.
//
// Printed as an **upper** bound and said so, where the documentation figure
// above it is a lower one. The count comes from canonicalising each side's
// order independently, which cannot tell an order nobody was asked for from
// one that is ours to get right, and nothing subtracts it from the raw
// figure.
func printReordered(o out, rep suite.Report) {
	if rep.Reordered == 0 {
		return
	}
	o.printf("                   %6d   of the first, differing only in an order — at most.\n", rep.Reordered)
	o.printf("                            An associative array's keys come back in the\n")
	o.printf("                            reference's own hash order, which is in nothing but\n")
	o.printf("                            its source. An upper bound, because the count cannot\n")
	o.printf("                            tell that from an order we should get right, and it\n")
	o.printf("                            corrects neither figure above it\n")
}
