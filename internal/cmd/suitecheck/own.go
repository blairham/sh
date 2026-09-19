// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/blairham/sh/internal/suite"
)

// binSet is the -bin flag in -own mode: dialect=path, repeated.
//
// Repeated rather than one binary, because every column runs at once. The
// whole point of our own suite is that four of the five dialects can never
// get a column from somebody else's work, and an instrument invoked one
// dialect at a time would let four of them go unrun without anybody noticing.
type binSet map[string]string

func (b binSet) String() string {
	var pairs []string
	for d, p := range b {
		pairs = append(pairs, d+"="+p)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func (b binSet) Set(v string) error {
	dialect, path, ok := strings.Cut(v, "=")
	if !ok || dialect == "" || path == "" {
		return fmt.Errorf("want dialect=path, got %q", v)
	}
	b[dialect] = path
	return nil
}

// runOwn grades every native column and then asks whether the references
// agree about core/.
func runOwn(ctx context.Context, root string, bins binSet, timeout time.Duration, jobs int, only string) int {
	fmt.Printf("our own suite — %s, committed and readable\n", root)
	fmt.Println("  The reference shell on this machine is the expectation; no expected output")
	fmt.Println("  is checked in. A .right file of ours would let us record our own bug as")
	fmt.Println("  correct, which is the one failure the oracle exists to prevent.")
	fmt.Println()
	fmt.Println("  That is also why a case the reference will not repeat is a failure here")
	fmt.Println("  and an exclusion in the fetched columns. With no expected output, a file")
	fmt.Println("  the reference answers two ways carries no expectation at all — and this")
	fmt.Println("  file is ours to fix. Every native file is asked twice for that reason,")
	fmt.Println("  not only the ones the two shells answered differently.")
	fmt.Println()

	opts := suite.Options{Timeout: timeout, Jobs: jobs, Only: names(only)}
	code := 0
	var refs []suite.Reference
	var reports []suite.Report
	var skipped []string
	var contained []string
	var crossless []string

	for _, s := range suite.OurColumns() {
		if s.NotYet != "" {
			skipped = append(skipped, fmt.Sprintf("%s — %s", s.Name, s.NotYet))
			continue
		}
		if missing := s.Missing(root); len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "suitecheck: %s claims %s under %s and they are not there\n",
				s.Name, strings.Join(missing, ", "), root)
			code = 1
			continue
		}
		if s.Contained() {
			// The reference is pinned inside an image, so the whole sweep
			// goes in there: see suite.RunContained. The binary is
			// cross-compiled there rather than taken from -own-bin, because
			// the one `make suite` built here is for this machine and the
			// image is Linux.
			rep, err := suite.RunContained(ctx, s, root, "./cmd/"+s.Dialect, opts)
			if err != nil {
				// Loud, and a column rather than a silence. A machine with
				// no container runtime legitimately cannot reach these, and
				// `ash: not run (no container runtime here)` has to be
				// impossible to mistake for `ash: agrees`.
				skipped = append(skipped, fmt.Sprintf("%s — %s", s.Name, err.Error()))
				continue
			}
			reports = append(reports, rep)
			contained = append(contained, s.Name)
			// Grading and cross-checking are two different questions, so a
			// contained column still supplies a cross-check reference where
			// this machine has one. Without this, containing a column takes
			// its reference out of suite.CrossCheck and its dialect tier out
			// of suite.OnlyHere — core/ from four references to three, ext/
			// from three to two, and the 21 files under bash/ unchecked — so
			// an absolute grading figure would have been bought with a
			// weaker tier claim. See suite.CrossHere for why this is a
			// statement on the column rather than a search: a path that
			// exists is not a reference.
			reference, here := s.CrossHereReference()
			if !here {
				crossless = append(crossless, s.Name)
				continue
			}
			refs = append(refs, suite.Reference{Name: s.Name, Path: reference})
			continue
		}
		reference, found := suite.Locate(s.Lookup)
		if !found {
			skipped = append(skipped, fmt.Sprintf("%s — no %s on this machine to be the reference; looked in %s",
				s.Name, s.Name, strings.Join(s.Lookup, ", ")))
			continue
		}
		named, ok := bins[s.Dialect]
		if !ok {
			skipped = append(skipped, fmt.Sprintf("%s — no -own-bin %s=… was given, so cmd/%s was not graded",
				s.Name, s.Dialect, s.Dialect))
			continue
		}
		// Resolved here, where the person who typed it is, and absolute:
		// every run gets a directory of its own to ruin, so a relative path
		// to the binary under test resolves against that directory instead.
		// It used to resolve to nothing and the column reported 0/10 strict
		// rather than an error — a harness fault wearing a shell's failure.
		bin, err := suite.Shell(named)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suitecheck: -own-bin %s=%s: %v\n", s.Dialect, named, err)
			code = 1
			continue
		}
		refs = append(refs, suite.Reference{Name: s.Name, Path: reference})
		rep, err := suite.Sweep(ctx, s, root, bin, reference, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suitecheck: %s: %v\n", s.Name, err)
			code = 1
			continue
		}
		rep.ReferenceVersion = s.Identify(ctx, reference)
		reports = append(reports, rep)
	}

	for _, rep := range reports {
		printOwnColumn(rep)
		if len(rep.CaseDefects()) > 0 {
			code = 1
		}
	}
	printOwnTable(reports)
	fmt.Println("  A tier is a claim, not filing. core/ is what a script may assume in any of")
	fmt.Println("  these shells. A cross-check runs a tier through the references alone and")
	fmt.Println("  asks whether they agree, which is the half no fetched suite can have:")
	fmt.Println("  grading our binary against one reference proves that dialect right, while")
	fmt.Println("  the references agreeing proves the construct common — the claim")
	fmt.Println("  docs/spec/shell-matrix.md makes in prose and nothing until now measured.")
	fmt.Println()
	fmt.Println("  ext/ is the same claim one step out: the ksh-family constructs the")
	fmt.Println("  substrate adopted when docs/spec/shell-matrix.md measured dash as the sole")
	fmt.Println("  holdout on 13 of 21 rows and excluded it. dash and ash do not run ext/, and")
	fmt.Println("  that absence is the measurement rather than an exemption — they are the")
	fmt.Println("  shells the boundary was drawn around.")
	fmt.Println()
	fmt.Println("  A per-dialect tier carries the opposite claim, and it is measured the")
	fmt.Println("  opposite way: core/ says every reference agrees, ksh/ says this is the")
	fmt.Println("  answer only ksh93 has. So its files are run under every reference here and")
	fmt.Println("  that column's own must be alone in what it wrote. A dialect file another")
	fmt.Println("  shell answers byte for byte is a core or an ext case filed in the wrong")
	fmt.Println("  directory, where it is graded against one shell instead of four.")
	fmt.Println()

	for _, name := range suite.Tiers {
		cross, err := suite.CrossCheck(ctx, root, name, refs, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suitecheck: the %s cross-check: %v\n", name, err)
			code = 1
			continue
		}
		printCross(cross)
		if len(cross.Unstable) > 0 {
			code = 1
		}
	}
	for _, col := range suite.OurColumns() {
		// A contained column is no longer skipped here. OnlyHere needs the
		// column's own reference among refs and reports nothing without it,
		// so the condition that matters is already the one it applies —
		// and where a contained column kept a reference on this machine,
		// its dialect tier keeps the check it had before it was pinned.
		if col.DialectTier() == "" {
			continue
		}
		own, err := suite.OnlyHere(ctx, root, col, refs, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "suitecheck: the %s/ only-here check: %v\n", col.DialectTier(), err)
			code = 1
			continue
		}
		printOwn(own)
		if len(own.Unstable) > 0 {
			code = 1
		}
	}
	printContained(contained, crossless)
	printCrossOmission(crossless)
	printOwnOmission(crossless)

	if len(skipped) > 0 {
		fmt.Println("  columns not run")
		for _, why := range skipped {
			fmt.Printf("    %s\n", wrap(why, "      "))
		}
		fmt.Println()
		fmt.Println("  A column that is not run is printed rather than dropped: a missing column")
		fmt.Println("  that says nothing reads as a column that passed.")
		fmt.Println()
	}
	return code
}

// printOwnColumn is one native column, and it names the cases that failed.
//
// Naming them is the difference between this and the fetched columns, and it
// is the only difference in what may be printed. These files are ours.
func printOwnColumn(rep suite.Report) {
	s := rep.Suite
	fmt.Printf("%s — %d files (%s)\n", s.Name, rep.Files, strings.Join(s.Dirs, "/, ")+"/")
	fmt.Printf("  reference  %s — %s\n", rep.Reference, rep.ReferenceVersion)
	// Immediately under the reference line, because it is a statement about
	// that line and because a caveat printed after the numbers is one a
	// reader has already formed an opinion without. See [suite.Suite.Lineage].
	if label, why := s.Lineage(rep.ReferenceVersion); why != "" {
		fmt.Printf("  %s\n", wrap(label+" — "+why, "    "))
	}
	fmt.Printf("  ours       %s\n", rep.Ours)
	if rep.Route != "" {
		// Said on every run of this column, because the claim is narrower
		// than the other four make: both shells ran inside an image, so this
		// pins the behavior of that image rather than of this machine.
		fmt.Printf("  reached    %s — both shells ran in there, on one copy of the files\n", rep.Route)
	}
	fmt.Printf("  parsed     %-9s %5.1f%%   our parser read the whole file\n",
		fmt.Sprintf("%d/%d", rep.Parsed, rep.Files), 100*rep.ParseRate())
	fmt.Printf("  strict     %-9s %5.1f%%   every byte and the status identical\n",
		fmt.Sprintf("%d/%d", rep.Strict, rep.Scored), 100*rep.StrictRate())
	fmt.Printf("  lines      %-9s %5.1f%%   longest common subsequence, by line\n",
		"", 100*rep.LineRate())
	if rep.OracleHung+rep.DialectHung > 0 {
		fmt.Printf("  not scored %d reference hung · %d ours hung\n",
			rep.OracleHung, rep.DialectHung)
	}
	if defects := rep.CaseDefects(); len(defects) > 0 {
		// Never "not scored", and never folded in with a hang. A fetched
		// suite's unstable file is an exclusion because nobody here may edit
		// it; ours is a bug in a file we wrote, and the run fails on it.
		fmt.Printf("  BAD CASES  %d of these files are not deterministic — %s\n",
			len(defects), strings.Join(defects, ", "))
		fmt.Println("             The reference answered one of them two different ways, so it")
		fmt.Println("             carries no expectation for anything to be graded against. Our")
		fmt.Println("             suite ships no expected output, which makes that a defect in")
		fmt.Println("             the case rather than an unlucky file: a pid, a clock, a")
		fmt.Println("             scheduling order, a path the harness did not normalize. Fix")
		fmt.Println("             the case. This is the one place our columns invert the")
		fmt.Println("             fetched rule, and it is why this run exits non-zero.")
		for _, line := range rep.CaseDefectReports() {
			// What moved, and not only which file. A suite file is hundreds
			// of lines, and the flake this was added for shows on about two
			// runs in fourteen on a machine nobody can log into — so the run
			// that catches it is the only thing that will ever say where.
			fmt.Printf("             %s\n", line)
		}
	}
	if rep.OracleFailed+rep.DialectFailed > 0 {
		// Loud, and never folded into the score. A run that did not start
		// agreed with nothing and disagreed with nothing.
		fmt.Printf("  NOT MEASURED %d reference · %d ours never started at all — a harness fault\n",
			rep.OracleFailed, rep.DialectFailed)
	}
	failed := rep.NotStrict()
	if len(failed) == 0 {
		fmt.Println("  every case agreed")
		fmt.Println()
		return
	}
	fmt.Println("  cases to fix")
	for _, c := range failed {
		switch {
		case !c.Result.Parsed:
			fmt.Printf("    %-28s our parser refused it: %s\n", c.Name, c.Result.Cause)
		case c.Result.DialectHung:
			fmt.Printf("    %-28s ours never finished — a hang we published\n", c.Name)
		case c.Result.OracleHung:
			fmt.Printf("    %-28s the reference never finished — a harness fault, not a finding\n", c.Name)
		case c.Result.Unstable:
			// Listed above as well, under BAD CASES, and deliberately: this
			// line is about the column's score and that one is about the
			// file. A reader scanning for work needs it in both places.
			fmt.Printf("    %-28s the case is not deterministic — fix the case, not the shell\n", c.Name)
		case c.Result.DialectFailed:
			fmt.Printf("    %-28s ours never started — a harness fault, not a finding\n", c.Name)
		case c.Result.OracleFailed:
			fmt.Printf("    %-28s the reference never started — a harness fault, not a finding\n", c.Name)
		default:
			fmt.Printf("    %-28s %d/%d lines · status %d, reference %d\n",
				c.Name, c.Result.Common, c.Result.Longest, c.Result.OurStatus, c.Result.RefStatus)
		}
	}
	fmt.Println()
}

// printOwnTable is the per-dialect baseline, on one screen.
//
// The number this campaign burns down is strict, per column. It is the harsh
// one on purpose: a case file is a few dozen assertions and one disagreement
// forfeits all of them, which is the right pressure for a suite we control —
// unlike a fetched file of hundreds, where it reads as catastrophe.
func printOwnTable(reports []suite.Report) {
	if len(reports) == 0 {
		return
	}
	fmt.Println("  per-dialect baseline")
	fmt.Printf("    %-8s %-9s %-9s %s\n", "column", "parsed", "strict", "lines")
	for _, rep := range reports {
		fmt.Printf("    %-8s %-9s %-9s %5.1f%%\n",
			rep.Suite.Name,
			fmt.Sprintf("%d/%d", rep.Parsed, rep.Files),
			fmt.Sprintf("%d/%d", rep.Strict, rep.Scored),
			100*rep.LineRate())
	}
	fmt.Println()
}

// printContained says which columns were graded inside an image, and which
// of the two questions that does and does not change.
//
// It is printed on every run that has one, because the claim a contained
// column makes is both stronger and narrower than the others': stronger
// because the reference is a pinned build rather than whatever the machine
// has, narrower because the figure is about that image. A column whose
// reference changed underneath the number is exactly what pinning it is for,
// and a reader who is not told which columns are pinned cannot tell the two
// kinds of figure apart.
func printContained(contained, crossless []string) {
	if len(contained) == 0 {
		return
	}
	out := map[string]bool{}
	for _, name := range crossless {
		out[name] = true
	}
	fmt.Printf("  graded inside a pinned image: %s\n", strings.Join(contained, ", "))
	fmt.Println("    both shells ran in there on one copy of the files, so the figure is the")
	fmt.Println("    same on a runner as on a laptop rather than a fact about the machine's")
	fmt.Println("    own build of the reference. That is what a WRONG BUILD banner says is")
	fmt.Println("    missing, and it is what #2291's per-column bar can be counted from.")
	var kept []string
	for _, name := range contained {
		if !out[name] {
			kept = append(kept, name)
		}
	}
	if len(kept) > 0 {
		fmt.Printf("    still a cross-check reference here: %s\n", strings.Join(kept, ", "))
		fmt.Println("      Grading and cross-checking ask different questions. Grading asks")
		fmt.Println("      whether our binary agrees with the build this column names, which is")
		fmt.Println("      why it is pinned. The cross-check asks whether the reference shells")
		fmt.Println("      on one machine agree with each other, which is a question about them")
		fmt.Println("      and needs them all in one place — so it keeps the binary on this")
		fmt.Println("      machine, pinned by nothing, exactly as it was before this column was")
		fmt.Println("      contained. Containing a column without this would have paid for an")
		fmt.Println("      absolute grading figure with a tier claim over fewer references.")
	}
	fmt.Println()
}

// printCrossOmission says which columns the cross-check could not include,
// and why it is a limit of the machine rather than a judgement about them.
//
// A column graded inside an image whose shell this machine does not have —
// BusyBox on a Mac is the whole of the list today — supplies no reference
// here at all. Its reference inside the image is on a different operating
// system from the others, so folding those bytes in would score a libc
// diagnostic and a coreutil as a disagreement between two shells, which is
// the confound the contained sweep was arranged to avoid one level down.
//
// Saying so is the point: a cross-check that listed four shells where five
// columns ran, with nothing explaining the difference, reads as a shell that
// agreed.
func printCrossOmission(crossless []string) {
	if len(crossless) == 0 {
		return
	}
	fmt.Printf("  not in the cross-check: %s\n", strings.Join(crossless, ", "))
	fmt.Println("    graded inside a container and with no binary of that shell on this")
	fmt.Println("    machine to stand in, so it supplies no reference here. Comparing the")
	fmt.Println("    bytes from in there against the references here would score a libc")
	fmt.Println("    diagnostic as a disagreement between two shells. The column is graded")
	fmt.Println("    against its own reference in there, where both sides match.")
	fmt.Println()
}

// printCross is the core claim, measured.
func printCross(cross suite.Cross) {
	if len(cross.Shells) < 2 {
		return
	}
	fmt.Printf("  %s/ agreement  %d/%d   the reference shells themselves wrote the same bytes\n",
		cross.Tier, cross.Agree, cross.Files)
	fmt.Printf("                  across %s\n", strings.Join(cross.Shells, ", "))
	for _, split := range cross.Split {
		var groups []string
		for _, g := range split.Groups {
			groups = append(groups, strings.Join(g, "+"))
		}
		fmt.Printf("    %-28s not core: %s\n", split.Name, strings.Join(groups, " ≠ "))
	}
	for _, name := range cross.Unstable {
		fmt.Printf("    %-28s not deterministic: a reference answered it two ways, so it can\n", name)
		fmt.Println("                                 make no claim about this tier at all. It is ours — fix it.")
		printMoved(cross.Moved[name])
	}
	fmt.Println()
}

// printOwn is one dialect tier's claim, measured.
func printOwn(own suite.Own) {
	if own.Files == 0 || len(own.Others) == 0 {
		return
	}
	fmt.Printf("  %s/ only-here  %d/%d   %s answered differently from every other reference\n",
		own.Tier, own.Alone, own.Files, own.Shell)
	fmt.Printf("                  held against %s\n", strings.Join(own.Others, ", "))
	for _, share := range own.Shared {
		fmt.Printf("    %-28s not only %s: %s wrote the same bytes\n",
			share.Name, own.Shell, strings.Join(share.With, ", "))
	}
	for _, name := range own.Unstable {
		fmt.Printf("    %-28s not deterministic: a reference answered it two ways, so it can\n", name)
		fmt.Println("                                 make no claim about this tier at all. It is ours — fix it.")
		printMoved(own.Moved[name])
	}
	fmt.Println()
}

// printMoved is which reference moved and what it wrote the second time.
func printMoved(why string) {
	if why == "" {
		return
	}
	const indent = "                                 "
	fmt.Printf("%s%s\n", indent, wrap(why, indent))
}

// printOwnOmission says why a column's own tier is not checked this way, for
// printCrossOmission's reason one step over: with no binary of that shell
// here, the references this tier would be held against are all on a different
// operating system from the one its reference runs on, so "alone" would be
// measuring two machines.
//
// It is the crossless list rather than every contained column, and that is
// the #3480 change in one line: a contained column that still has a binary
// here keeps its only-here check, because that check runs on this machine and
// never went near the image.
func printOwnOmission(crossless []string) {
	for _, name := range crossless {
		col, ok := suite.FindOurs(name)
		if !ok || col.DialectTier() == "" {
			continue
		}
		fmt.Printf("  %s/ has no only-here check: %s's reference is inside a container, so\n",
			col.DialectTier(), name)
		fmt.Println("    holding it against the references on this machine would score an")
		fmt.Println("    operating system as a shell. The tier is still graded in there against")
		fmt.Println("    its own reference, which is the half that does not need a comparison.")
		fmt.Println()
	}
}
