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
// real shells do with 162 snippets; pointing it at our own binary turns every
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
	SameStatus int
	Total      int
	Missing    []string
	NotBuilt   bool
}

// RunConformance runs every case through the binary at path and compares it
// with the named reference shell.
//
// Cases the reference shells reject are skipped: what an implementation does
// with input that is not valid shell is a separate question from whether it
// agrees about input that is.
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
		if c.SyntaxError {
			continue
		}
		want := Exec(ctx, ref, c)
		got := Exec(ctx, ours, c)
		ok := want.Output == got.Output && want.Status == got.Status
		rep.Matches = append(rep.Matches, Match{CaseID: c.ID, Want: want, Got: got, OK: ok})
		rep.Total++
		switch {
		case ok:
			rep.Passed++
		case want.Status == got.Status:
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
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "panel members absent here: %s\n", strings.Join(r.Missing, ", "))
	}
	if !verbose {
		return b.String()
	}
	b.WriteString("\nnot matching:\n")
	for _, m := range r.Matches {
		if m.OK {
			continue
		}
		fmt.Fprintf(&b, "  %s\n    want %q (status %d)\n    got  %q (status %d)\n",
			m.CaseID, m.Want.Output, m.Want.Status, m.Got.Output, m.Got.Status)
	}
	return b.String()
}
