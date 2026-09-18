// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

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
	// ByProbe says Version is a fingerprint taken with [Suite.AgainstProbe]
	// rather than a build string the shell printed.
	//
	// The two are used identically — printed in the header, matched against
	// AgainstReport — and they are still not the same kind of fact, so the
	// report may not present a fingerprint as though the shell had named
	// itself. A reader who sees `esc=4 pipefail-listed=n` where a version
	// belongs has to be told that is a measurement, or the next person to
	// change the probe will think they are editing a version string.
	ByProbe bool
}

// String is the build for a reader, and it never returns something that could
// be mistaken for a version.
func (b Build) String() string {
	if !b.Known {
		return "could not determine — this shell answers no version probe"
	}
	if b.ByProbe {
		return b.Version + " — a fingerprint, because this shell answers no version probe"
	}
	return b.Version
}

// Version asks a reference shell what build it is.
//
// The spellings are internal/oracle's, called rather than copied, and as of
// #3148 so is the gate: [oracle.LooksLikeABuild]. A second list of ways to
// ask a shell its version is what let this package go without the ksh one for
// as long as it did, and a second copy of the rule that says which answers
// count would have been the same mistake one layer up — the repair for a
// helper that has drifted from the one it was copied from is to fold it back,
// not to patch both.
//
// The rule is now applied on both sides of that call and that is not a
// duplicate, because it is one definition read twice: the probe asks whether
// a line identifies the binary, and this asks whether a report may print the
// line as a build. The probe's `unknown` for a shell that answered no
// spelling fails the same rule, carrying no build number, so there is no
// second sentinel to keep in step either.
func Version(ctx context.Context, shell string) Build {
	line := firstLine(oracle.Version(ctx, shell))
	if !oracle.LooksLikeABuild(line) {
		return Build{}
	}
	return Build{Version: line, Known: true}
}

// Identify is what a report must call rather than [Version]: it asks the
// shell what it is, and where the shell will not say, it measures.
//
// # The column that forced this
//
// dash answers no version probe in any spelling — not `--version`, not
// `--help`, not `${.sh.version}` — so [Version] returns Known == false for
// it on every machine, `Against` was left empty at its entry for that
// reason, and [Suite.Lineage] therefore had nothing to raise a notice from.
// The consequence was not theoretical. The dash column is the one CI report
// with no banner on it, which is exactly why the previous reading of #2291
// called it "the only unexcused per-column strict gap" — and both of the
// files in that gap are the *distribution's patch* rather than anything
// ours:
//
//   - `printf 'a\eZ'` is three bytes under Debian's dash and four under
//     upstream 0.5.12 and Apple's dash-16 alike;
//   - `set -o` lists nineteen options under Debian's and seventeen under
//     both of the others, `privileged` and `pipefail` being the additions.
//
// Two builds of one upstream version disagreeing is what says it is a patch
// and not a version the cases are behind on. Measured 2026-09-16: every one
// of the 52 files this column runs is byte-identical under Apple's dash-16
// and under upstream 0.5.12 built from source on the same machine, once the
// shell's own path and the per-run directory are normalized the way [Sweep]
// normalizes them.
//
// # Why a fingerprint is allowed to stand in for a version
//
// Because the question the notice asks is not "what release is this" — it is
// "is the shell in front of me the one this column's cases were measured
// against". A version string answers that by proxy and a fingerprint answers
// it directly, which is the better instrument for a build whose divergence
// is a patch rather than a release. It also cannot be spoofed by a refusal
// the way #3135's usage line was: the probe is scored on what the shell
// *did*, and a shell that refuses it prints no fingerprint at all.
//
// The rule from [oracle.LooksLikeABuild] carries over in the same form. An answer
// has to look like an answer: a non-zero status, an empty line, or anything
// on standard error instead of standard output is not an identification, and
// the column says it could not tell rather than printing a refusal where a
// build belongs.
func (s Suite) Identify(ctx context.Context, shell string) Build {
	if b := Version(ctx, shell); b.Known {
		return b
	}
	if s.AgainstProbe == "" {
		return Build{}
	}
	out, ok := fingerprint(ctx, shell, s.AgainstProbe)
	if !ok {
		return Build{}
	}
	return Build{Version: out, Known: true, ByProbe: true}
}

// fingerprint runs a column's identifying probe through the reference and
// returns its first line.
//
// Standard error is dropped rather than folded in, which is the deliberate
// opposite of how a suite file is run: a file is run with the two streams
// joined because their interleaving is part of what is being compared, and a
// probe is run with them apart because a diagnostic wearing a fingerprint's
// clothes is #3135 again.
func fingerprint(ctx context.Context, shell, script string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-c", script)
	// The environment a probe of a shell has to have: nothing of the
	// machine's, and C, because a fingerprint that moved with LANG would be
	// measuring the machine rather than the build.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "LANG=C"}
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	line := strings.TrimSpace(firstLine(out.String()))
	return line, line != ""
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
	here := "the shell here is " + b.Version
	if b.ByProbe {
		here = "the shell here answers " + b.Version
	}
	return "WRONG BUILD", "this column is graded against " + s.Against +
		" and " + here + ". The numbers below are that build's " +
		"answers, not this column's: where the two shells word a diagnostic differently " +
		"the difference scores against us and is not ours to fix. A delta across one pull " +
		"request on this machine still means something; the absolute figure does not."
}
