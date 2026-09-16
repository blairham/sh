// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"regexp"
	"strings"

	"github.com/blairham/sh/internal/oracle"
)

// What a reference shell says it is, and what a column needs it to be.
//
// # The failure this file exists for
//
// Every number in a report is a comparison against whichever shell was on the
// machine, so the report has always printed that shell's build beside its
// column. What it printed it from was a second copy of internal/oracle's
// probe that had lost two halves of it, and both halves mattered:
//
//   - the ksh spelling. ksh93 answers neither `--version` nor `--help`; it
//     answers `${.sh.version}`, which internal/oracle has asked since #1032
//     and this package never did.
//   - the exit status. The `--help` fallback took any non-empty output, so a
//     shell that *refused* the probe had its refusal printed as its build.
//
// Measured here before the fix, the two columns whose references answer no
// version flag both printed a refusal where a build belongs:
//
//	ksh    Usage: ksh [ options ] [arg ...]
//	dash   /bin/dash: 0: Illegal option --
//
// The ksh line is the one that cost something. It reads like a build string
// at a glance, so nobody noticed for as long as the job has existed that the
// CI runner's ksh is ksh93u+m 1.0.8 — a fork, and twelve years on from the
// AT&T 93u+ 2012-08-01 build dialect/ksh is written against and the corpus
// was recorded from. The entire nineteen-line gap in that column's CI number
// is 93u+m's rewritten refusals; every accepting row agrees. A column was
// being discounted in public and the instrument's own header said nothing,
// because a failed probe could wear a successful one's clothes (#3135).
//
// # Two rules, and they are separate on purpose
//
//  1. An answer has to look like an answer, or the column reports that it
//     could not tell. Never a sentence that could be mistaken for a build.
//  2. A column names the build it is written against, and the report says so
//     when the shell in front of it is a different one.
//
// The second is what generalizes past ksh, which is the reason to write it
// rather than a special case for the loud column. The runner's bash is 5.2.21
// against a suite measured from 5.3.20 and its zsh is 5.9 against 5.9.2, and
// #3113 measured seven `0 / 2` rows on CI that do not exist on the machine
// the baselines were taken on for exactly that reason. Version skew is
// allowed here — these jobs are report-only and a delta across one pull
// request on one runner is what they are for — but it may not be invisible,
// because a reader who does not know the column is discounted reads the
// discount as our bug.

// Build is what a reference shell answered when asked what it is.
type Build struct {
	// Version is the build string the shell printed. Empty unless Known.
	Version string
	// Known says a probe answered with something that is a build string.
	//
	// A shell that refuses every spelling and a shell that answers one by
	// printing its usage are both Known == false, and the difference between
	// them is not one a report can act on: neither identified the binary.
	// What matters is that the report cannot print the second as though it
	// were an identification, which is the whole of #3135.
	Known bool
}

// String is the build for a reader, and it never returns something that could
// be mistaken for a version.
func (b Build) String() string {
	if !b.Known {
		return "could not determine — this shell answers no version probe"
	}
	return b.Version
}

// Version asks a reference shell what build it is.
//
// The spellings are internal/oracle's, called rather than copied. A second
// list of ways to ask a shell its version is what let this package go without
// the ksh one for as long as it did, and the repair for a helper that has
// drifted from the one it was copied from is to fold it back, not to patch
// both. What is added here is the gate — see [looksLikeABuild].
func Version(ctx context.Context, shell string) Build {
	line := firstLine(oracle.Version(ctx, shell))
	if !looksLikeABuild(line) {
		return Build{}
	}
	return Build{Version: line, Known: true}
}

// buildNumber is the mark of an answer: a build string names a build, so it
// carries a release number or the date of one. Every build string any column
// here has ever reported does — `5.3.20`, `5.9.2`, `93u+ 2012-08-01`,
// `v1.37.0` — and no refusal any of them prints does.
var buildNumber = regexp.MustCompile(`[0-9]+\.[0-9]+|[0-9]{4}-[0-9]{2}-[0-9]{2}`)

// looksLikeABuild is the gate: is this line an identification, or is it the
// shell declining to give one?
//
// Deliberately a rule about what an answer has rather than a list of the
// refusals seen so far. A blocklist is beaten by the first refusal nobody has
// met yet, and this rule can only ever be too strict — too strict is a column
// printing that it could not tell, which is the honest failure and the one a
// reader acts on correctly. Too loose is #3135.
func looksLikeABuild(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !buildNumber.MatchString(line) {
		return false
	}
	// A usage line carrying a number anyway — a path with a version in it, a
	// bracketed option that takes one — is still a refusal. This is the
	// shape #3135 was: ksh's `--help` exits zero and its first line is
	// usage. Folding the probe onto the oracle's means ksh no longer reaches
	// `--help` at all, and this stays because the next shell to answer only
	// `--help` will reach it.
	return !strings.HasPrefix(strings.ToLower(line), "usage:")
}

// Lineage is what the report must say about the build a column was actually
// graded against: a label for the report's left column and the sentence
// beside it, both empty when there is nothing to say.
//
// Nothing to say is the common case and it has to stay quiet, or the notice
// that means something is one more line nobody reads.
func (s Suite) Lineage(b Build) (label, why string) {
	switch {
	case s.Against == "":
		// A column that names no expected build cannot have one violated.
		// [TestTheColumnsWhoseReferenceReportsABuildNameIt] is what keeps
		// that from being a way to opt out of the check.
		return "", ""
	case !b.Known:
		return "UNIDENTIFIED", "this column is graded against " + s.Against +
			", and the shell here would not say what it is. Whether these numbers were " +
			"taken against that build is not recorded, so treat them as a delta on this " +
			"machine and not as a figure to quote."
	case strings.Contains(strings.ToLower(b.Version), s.AgainstReport):
		return "", ""
	}
	return "WRONG BUILD", "this column is graded against " + s.Against +
		" and the shell here is " + b.Version + ". The numbers below are that build's " +
		"answers, not this column's: where the two shells word a diagnostic differently " +
		"the difference scores against us and is not ours to fix. A delta across one pull " +
		"request on this machine still means something; the absolute figure does not."
}
