// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Conformance grades an implementation against a reference shell over the
// whole corpus.
//
// This is what the harness was built for. The corpus already records what six
// real shells do with the whole corpus; pointing it at our own binary turns every
// one of them into a conformance test, with no new cases to write and no
// expectations to maintain by hand.
//
// It is deliberately separate from the drift golden. Drift asks whether the
// *panel* still behaves as recorded, and mixing in a column that is expected
// to fail would make that check useless while the implementation is young.

// Match is one case's verdict.
type Match struct {
	CaseID string
	Want   Result
	Got    Result
	OK     bool

	// Relaxed marks a verdict that needed Case.GradedOnRefusal to be a pass:
	// the two sides refused alike and worded it differently, and an exact
	// comparison would have called that a divergence.
	//
	// It is recorded per match rather than only counted, because a reader has
	// to be able to see *which* rows were forgiven. A relaxation nobody can
	// enumerate is indistinguishable from a score that is wrong.
	Relaxed bool
}

// Report is the outcome of a conformance run.
type Report struct {
	Against string
	Matches []Match
	Passed  int
	// SameStatus counts cases that agree about what *happened* — the same
	// exit status — while disagreeing about the words. Diagnostics are not
	// specified by anything and no two shells word them alike, so an
	// exact-output score understates behavioral agreement and this says by
	// how much.
	//
	// Now that the two streams are recorded apart, a finer claim than this
	// one is available to whatever grades a refusal rather than its wording:
	// same status, same standard output, and a non-empty standard error on
	// both sides is "it complained, on stderr, and exited nonzero" stated
	// precisely rather than approximated. Nothing here makes that judgment
	// yet; the record now carries what it would need.
	SameStatus int

	// Relaxed counts the passes that needed Case.GradedOnRefusal. It is not a
	// second score but a discount on the first one, and it is printed for
	// that reason: a grading relaxation that does not appear in the report is
	// a way to make a diverging case look green, which is the thing this mode
	// must not become.
	Relaxed int

	Total    int
	Missing  []string
	NotBuilt bool
}

// RunConformance runs every case through the binary at path and compares it
// with the named reference shell.
//
// Cases the reference shells reject are skipped: what an implementation does
// with input that is not valid shell is a separate question from whether it
// agrees about input that is.
// graded reports whether a case can grade an implementation.
//
// Only one thing disqualifies a case: a reference that answers differently on
// different runs, which would move the score without the implementation having
// changed.
//
// SyntaxError used to disqualify one too, and should not have. A rejection is
// as deterministic as an acceptance, and its wording is exactly what the
// Diagnostics vector exists for — so excluding those cases left a whole vector
// ungraded and a report of 100% silent about it. Nine real gaps were hiding
// behind it when this was changed.
func graded(c Case) bool { return !c.ReferenceRaces }

// matches reports whether the implementation did what the reference did.
//
// Each stream on its own: matching a merge would let an implementation that
// writes its diagnostics to standard output score as agreeing with a shell
// that writes them to standard error.
//
// And the end, which is a status *or* a signal rather than only a status. A
// process a signal killed has no exit status, so both sides answer -1 for it
// — which made a shell killed by SIGINT, a shell killed by SIGKILL and a
// shell that hung until the harness gave up all score as the same behavior.
func matches(want, got Result) bool {
	return want.Stdout == got.Stdout && want.Stderr == got.Stderr && sameOutcome(want, got)
}

// refused reports whether a run is a refusal: it ended badly, and it said so.
//
// Both halves are required, and the second is the one the merged capture
// could not have asked for. "Exited nonzero" alone is any failure, including
// a script that ran and whose last command was false; a shell that *declined*
// also writes a diagnostic, and a diagnostic belongs on standard error. So
// this is the precise form of a claim the harness could previously only
// approximate — and it is a claim about behavior rather than about wording,
// which is what makes it safe to grade on.
//
// A timeout is not a refusal. A shell that never finished did not decline
// anything; it is the one outcome that must never be forgiven, because the
// case that hangs is the case that has stopped measuring.
func refused(r Result) bool {
	return !r.TimedOut && (r.Status != 0 || r.Signal != 0) && r.Stderr != ""
}

// matchesRefusal grades a case on the refusal rather than on its wording.
//
// It forgives exactly one thing: the words of the diagnostic. Everything the
// exact comparison asks for is still asked for — the same outcome, and the
// same standard output, byte for byte — and two requirements are *added* that
// the exact comparison does not make, namely that both sides actually refused.
//
// That is what keeps it from becoming a way to make a divergence look green.
// The mode is not "compare less"; it is "compare a different, still-falsifiable
// thing". Three ways it fails where a loose reading of "graded on the refusal"
// would pass:
//
//   - The reference did not refuse. Then the flag is on a case that is not a
//     refusal at all, and the verdict is a failure rather than a free pass —
//     a misused flag has to be louder than a correct one, not quieter.
//   - We did not refuse. Running what a shell declined to run is the whole
//     bug this mode exists to pin, so it cannot be the thing the mode hides.
//   - We refused silently. Saying nothing is not a wording difference, and an
//     implementation that exits 2 with an empty standard error has not
//     diagnosed anything.
//
// Standard output stays exact deliberately, and it costs something: bash
// answers `-cecho hi` by writing its whole `set -o` table to standard output,
// so a case pinning that shape reports a real gap against a bash reference
// until we write the table too. That is the right answer. Dropping the stream
// would forgive a shell that *ran* the command string, which is precisely the
// divergence the case was written to catch.
func matchesRefusal(want, got Result) bool {
	return refused(want) && refused(got) && sameOutcome(want, got) && want.Stdout == got.Stdout
}

// verdict grades one case, which is the only place the grading mode is read.
//
// It reports whether the two agreed and whether the agreement needed the
// relaxation. A case that matches exactly is never counted as relaxed, even
// when it carries the flag: the count has to mean "this much is currently
// being forgiven", or it measures the corpus's labeling instead of the
// implementation's agreement.
func verdict(c Case, want, got Result) (ok, relaxed bool) {
	if matches(want, got) {
		return true, false
	}
	if c.GradedOnRefusal && matchesRefusal(want, got) {
		return true, true
	}
	return false, false
}

// sameOutcome reports whether two runs ended the same way, ignoring what they
// printed. It is what "agreed about what happened but not about the wording"
// means, and no two shells word a diagnostic alike.
func sameOutcome(want, got Result) bool {
	return want.Status == got.Status && want.Signal == got.Signal && want.TimedOut == got.TimedOut
}

func RunConformance(ctx context.Context, path, against string, args []string, cases []Case) (*Report, error) {
	if path == "" {
		return &Report{NotBuilt: true}, nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no binary at %s: %w", path, err)
	}

	found, missing := Resolve(ctx)
	var ref Found
	for _, f := range found {
		if f.Name == against {
			ref = f
		}
	}
	if ref.Path == "" {
		return nil, fmt.Errorf("reference shell %q is not installed", against)
	}

	ours := Found{
		Shell: Shell{
			Name: "ours",
			// Whatever flags the binary needs to be the shell it is being
			// graded against. The core driver takes -dialect; a dialect
			// binary already is one and takes nothing.
			Args: args,
			Why:  "the implementation under test",
		},
		Path: path,
	}

	rep := &Report{Against: against, Missing: missing}
	for _, c := range cases {
		if !graded(c) {
			continue
		}
		want := Exec(ctx, ref, c)
		got := Exec(ctx, ours, c)
		ok, relaxed := verdict(c, want, got)
		rep.Matches = append(rep.Matches, Match{CaseID: c.ID, Want: want, Got: got, OK: ok, Relaxed: relaxed})
		rep.Total++
		switch {
		case ok:
			rep.Passed++
			if relaxed {
				rep.Relaxed++
			}
		case sameOutcome(want, got):
			rep.SameStatus++
		}
	}
	sort.Slice(rep.Matches, func(i, j int) bool { return rep.Matches[i].CaseID < rep.Matches[j].CaseID })
	return rep, nil
}

// Summary renders the report.
//
// It lists what does *not* match rather than what does, because the passing
// set is a number and the failing set is the work.
func (r *Report) Summary(verbose bool) string {
	if r.NotBuilt {
		return "no binary under test; build one and pass -bin\n"
	}
	var b strings.Builder
	pct := 0.0
	if r.Total > 0 {
		pct = 100 * float64(r.Passed) / float64(r.Total)
	}
	fmt.Fprintf(&b, "conformance against %s: %d/%d (%.0f%%)\n", r.Against, r.Passed, r.Total, pct)
	if r.SameStatus > 0 {
		behav := 100 * float64(r.Passed+r.SameStatus) / float64(r.Total)
		fmt.Fprintf(&b, "  plus %d agreeing on the exit status but not the wording — %.0f%% behavioral\n",
			r.SameStatus, behav)
	}
	if r.Relaxed > 0 {
		// Said in the summary and not only under -v, because this is the part
		// of the score that was not earned by an exact match. A reader who
		// sees only the percentage should still be told how much of it rests
		// on a relaxation, and which cases those are is one flag away.
		fmt.Fprintf(&b, "  of which %d passed on the refusal rather than the wording (-v lists them)\n", r.Relaxed)
	}
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "panel members absent here: %s\n", strings.Join(r.Missing, ", "))
	}
	if !verbose {
		return b.String()
	}
	if r.Relaxed > 0 {
		b.WriteString("\ngraded on the refusal, not the wording:\n")
		for _, m := range r.Matches {
			if m.Relaxed {
				fmt.Fprintf(&b, "  %s\n    want %s\n    got  %s\n", m.CaseID, describe(m.Want), describe(m.Got))
			}
		}
	}
	b.WriteString("\nnot matching:\n")
	for _, m := range r.Matches {
		if m.OK {
			continue
		}
		fmt.Fprintf(&b, "  %s\n    want %s\n    got  %s\n", m.CaseID, describe(m.Want), describe(m.Got))
	}
	return b.String()
}
