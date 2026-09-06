// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Running the corpus under a policy, which is the payoff the sandbox was
// built for and the half that was still open.
//
// The gate has unit tests, and unit tests of a boundary are written by
// somebody who already knows where the boundary is. The corpus is fourteen
// hundred cases written for other reasons entirely, by somebody thinking
// about `printf` and `trap` and `${x%%y}` — so running it gated exercises the
// gate from every direction at once, and a case whose answer *changes* under
// the policy is a case that touches the filesystem, which is information
// nothing else here produces.
//
// # This reports, it does not grade
//
// A conformance run that failed on a denial would be failing on the policy
// rather than on compatibility, and the two are different questions: the
// score against bash says whether we are a shell, and this says what the
// boundary is doing. So this is its own mode with its own number, and both
// sides of it are our own binary — no reference shell is consulted at all,
// which is what makes the number the same on a machine with a thin panel.

// ContainmentPolicy is the policy the corpus runs under, as a policy file.
//
// The posture is the one thing here worth arguing about, and two were tried.
//
// **Deny everything, allow nothing** is the tempting one: it is the strongest
// statement, it needs no argument about what to allow, and it does put every
// case through the gate. Measured over the corpus it moves 497 of 1397 cases,
// because most of them run a program — and a wall of differences is not a
// signal, it is a second corpus nobody will read. It stays available by
// naming a policy file; it is not the default.
//
// **Writes are confined to the directory the harness gave the case** is what
// this is, and it is the posture that makes a *claim*: a corpus case writes
// where it was put and nowhere else. Measured, it moves 18 cases, each of
// which can be explained — which is a number somebody can watch.
//
// Reads and execs are deliberately not confined. A case reads /dev/null and
// runs /usr/bin/true, and confining those would grade the corpus on the
// policy again; more to the point an allowed exec is outside the boundary the
// moment it starts, so a policy refusing reads while permitting programs is
// telling itself a story. What can honestly be said is about writes, and it
// is said exactly.
//
// The temporary directory is named rather than assumed because it is not the
// same on two platforms — `/var/folders/…` on a Mac and `/tmp` on Linux — so
// a checked-in file could not say it and this is generated per run.
func ContainmentPolicy(scratch string) string {
	return strings.Join([]string{
		"version 1",
		"default allow",
		"default deny write",
		"allow write " + filepath.Clean(scratch) + "/**",
		// The one write every shell makes that is not in a scratch
		// directory. `> /dev/null` is not a change to anything.
		"allow write /dev/**",
		"",
	}, "\n")
}

// WriteContainmentPolicy puts the policy somewhere the shell under test can
// read it, and returns the path.
//
// Named for the run rather than fixed, because two graders often run at once
// in this repository and a fixed name lets one rewrite the other's file
// mid-sweep — the same hazard the Makefile's BINDIR comment records about
// binaries.
func WriteContainmentPolicy() (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "oracle-policy-")
	if err != nil {
		return "", func() {}, err
	}
	path = filepath.Join(dir, "corpus.policy")
	if err := os.WriteFile(path, []byte(ContainmentPolicy(os.TempDir())), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

// Change is one case that answered differently with the policy on.
type Change struct {
	CaseID       string
	Plain, Gated Result
	// Worded marks a change that is only a change of wording: the case wrote
	// the same standard output and ended the same way, and only the
	// diagnostic differs.
	//
	// It is the difference between "the policy refused something that was
	// going to fail anyway, and said so in its own words" and "the policy
	// changed what happened". Both are worth listing and only the second is
	// worth investigating, so a report that counted them together would bury
	// the three cases that matter under the fifteen that do not.
	Worded bool
}

// worded reports whether two results differ only in what was printed to
// standard error.
func worded(before, after Result) bool {
	return before.Stdout == after.Stdout && sameOutcome(before, after)
}

// GatedReport is the outcome of a gated sweep.
type GatedReport struct {
	// Policy is the file the gated column was run under, so a reader can see
	// what the number is a number about.
	Policy   string
	Total    int
	Changed  []Change
	NotBuilt bool
}

// Behaved counts the changes where something other than the wording moved.
func (r *GatedReport) Behaved() int {
	n := 0
	for _, c := range r.Changed {
		if !c.Worded {
			n++
		}
	}
	return n
}

// RunGated runs every case twice through the same binary — once as it is, and
// once with a policy — and reports which cases answered differently.
//
// The comparison is the whole Result and not the verdict against a reference.
// A case that agrees with bash both ways and prints something different in
// between has still touched the filesystem, and that is what this is for; a
// verdict comparison would round it away.
func RunGated(ctx context.Context, path string, args []string, policy string, cases []Case) (*GatedReport, error) {
	if path == "" {
		return &GatedReport{NotBuilt: true}, nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no binary at %s: %w", path, err)
	}
	if _, err := os.Stat(policy); err != nil {
		return nil, fmt.Errorf("no policy at %s: %w", policy, err)
	}

	shell := func(extra ...string) Found {
		return Found{
			Shell: Shell{
				Name: "ours",
				Args: append(append([]string(nil), args...), extra...),
				Why:  "the implementation under test",
			},
			Path: path,
		}
	}
	plain, gated := shell(), shell("-policy", policy)

	rep := &GatedReport{Policy: policy}
	for _, c := range cases {
		if !graded(c) {
			continue
		}
		rep.Total++
		before := Exec(ctx, plain, c)
		after := Exec(ctx, gated, c)
		if before == after {
			continue
		}
		rep.Changed = append(rep.Changed, Change{
			CaseID: c.ID, Plain: before, Gated: after, Worded: worded(before, after),
		})
	}
	sort.Slice(rep.Changed, func(i, j int) bool { return rep.Changed[i].CaseID < rep.Changed[j].CaseID })
	return rep, nil
}

// Summary renders the gated report.
//
// It leads with what did *not* change, because that is the claim: the policy
// is on, the corpus ran, and this much of it behaved identically. The rest is
// the work, and it is listed rather than counted for the reason the
// conformance report lists its failures — a difference nobody can enumerate
// is indistinguishable from a number that is wrong.
func (r *GatedReport) Summary(verbose bool) string {
	if r.NotBuilt {
		return "no binary under test; build one and pass -bin\n"
	}
	var b strings.Builder
	unchanged := r.Total - len(r.Changed)
	fmt.Fprintf(&b, "gated corpus: %d/%d unchanged under %s\n", unchanged, r.Total, r.Policy)
	fmt.Fprintf(&b, "  %d case(s) answered differently with the policy on, %d of them in more than the wording\n",
		len(r.Changed), r.Behaved())
	if !verbose {
		if len(r.Changed) > 0 {
			b.WriteString("  -v lists them\n")
		}
		return b.String()
	}
	// The behavioral ones first and under their own heading, because a list
	// that opened with fifteen reworded diagnostics is a list whose three
	// real entries nobody reaches.
	for _, heading := range []struct {
		title  string
		worded bool
	}{
		{"what happened changed:", false},
		{"only the wording changed:", true},
	} {
		shown := false
		for _, c := range r.Changed {
			if c.Worded != heading.worded {
				continue
			}
			if !shown {
				fmt.Fprintf(&b, "\n%s\n", heading.title)
				shown = true
			}
			fmt.Fprintf(&b, "  %s\n    plain %s\n    gated %s\n", c.CaseID, describe(c.Plain), describe(c.Gated))
		}
	}
	return b.String()
}
