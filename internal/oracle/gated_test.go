// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// TestContainmentPolicyMeansWhatItSays parses the generated policy with the
// real parser and asks it the questions the comment on it makes claims about.
//
// Asserted through internal/policy rather than by comparing the text, and
// that is the point: a policy file is a claim about decisions, and a test
// that compared strings would pass for a file whose rules had stopped meaning
// anything — a mistyped selector parses, matches nothing, and reads exactly
// like a rule that is being obeyed.
func TestContainmentPolicyMeansWhatItSays(t *testing.T) {
	scratch := "/scratch"
	p, err := policy.Parse(strings.NewReader(ContainmentPolicy(scratch)))
	if err != nil {
		t.Fatalf("the generated policy does not parse: %v", err)
	}
	for _, tc := range []struct {
		name string
		a    interp.Action
		want interp.Decision
	}{
		{"a write in the scratch directory", interp.Action{Kind: interp.ActionOpen, Path: "/scratch/out", Write: true}, interp.Allow},
		{"a write deeper in it", interp.Action{Kind: interp.ActionOpen, Path: "/scratch/a/b/out", Write: true}, interp.Allow},
		{"a write outside it", interp.Action{Kind: interp.ActionOpen, Path: "/etc/passwd", Write: true}, interp.Deny},
		{"a write to a neighbor named alike", interp.Action{Kind: interp.ActionOpen, Path: "/scratchpad/out", Write: true}, interp.Deny},
		{"a write to /dev/null", interp.Action{Kind: interp.ActionOpen, Path: "/dev/null", Write: true}, interp.Allow},
		{"a read outside it", interp.Action{Kind: interp.ActionOpen, Path: "/etc/passwd"}, interp.Allow},
		{"listing a directory outside it", interp.Action{Kind: interp.ActionReadDir, Path: "/etc"}, interp.Allow},
		{"running a program", interp.Action{Kind: interp.ActionExec, Path: "/usr/bin/true"}, interp.Allow},
		{"signaling a process", interp.Action{Kind: interp.ActionSignal, PID: 1}, interp.Allow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Allow(context.Background(), tc.a); got != tc.want {
				t.Errorf("Allow(%v) = %v, want %v", tc.a, got, tc.want)
			}
		})
	}
}

// TestWriteContainmentPolicyNamesThisMachinesScratchDirectory. The temp base
// is `/var/folders/…` on a Mac and `/tmp` on Linux, which is why the file is
// generated per run rather than committed.
func TestWriteContainmentPolicyNamesThisMachinesScratchDirectory(t *testing.T) {
	path, cleanup, err := WriteContainmentPolicy()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), filepath.Clean(os.TempDir())+"/**") {
		t.Errorf("the policy does not name %s:\n%s", os.TempDir(), body)
	}
	cleanup()
	if _, err := os.Stat(path); err == nil {
		t.Error("cleanup left the policy behind")
	}
}

// fakeShell is a shell for the purposes of this file: it answers three
// snippets, and it can tell whether it was handed a policy.
//
// A real shell will not do here, because the property under test is what the
// harness does with the *difference* between two columns, and a shell that
// behaves identically either way can only prove the half where nothing
// changed. This one is written to change in each of the ways the report
// distinguishes.
func fakeShell(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-shell")
	const body = `#!/bin/sh
gated=no
if [ "$1" = "-policy" ]; then gated=yes; shift 2; fi
case "$2" in
  same)     echo same ;;
  louder)   echo out; echo "complaint-$gated" >&2 ;;
  *)        echo "out-$gated" ;;
esac
`
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGatedReportsOnlyWhatThePolicyChanged.
//
// Three cases and three verdicts: one that answers the same either way and is
// not reported at all, one whose standard output moves, and one that prints
// the same thing and complains differently. The last is the split the summary
// is built on — a reworded refusal is not a change in what happened, and a
// report that counted the two together would bury the one that matters.
func TestGatedReportsOnlyWhatThePolicyChanged(t *testing.T) {
	shell := fakeShell(t)
	policyFile := filepath.Join(t.TempDir(), "p.policy")
	if err := os.WriteFile(policyFile, []byte(ContainmentPolicy(t.TempDir())), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []Case{
		{ID: "unchanged", Snippet: "same"},
		{ID: "output-moved", Snippet: "quieter"},
		{ID: "worded-differently", Snippet: "louder"},
	}

	rep, err := RunGated(context.Background(), shell, nil, policyFile, cases)
	if err != nil {
		t.Fatalf("RunGated: %v", err)
	}
	if rep.Total != 3 {
		t.Errorf("Total = %d, want every case counted", rep.Total)
	}
	if len(rep.Changed) != 2 {
		t.Fatalf("changed = %d (%v), want the two that moved and not the one that did not",
			len(rep.Changed), rep.Changed)
	}
	byID := map[string]Change{}
	for _, c := range rep.Changed {
		byID[c.CaseID] = c
	}
	if _, ok := byID["unchanged"]; ok {
		t.Error("a case that answered identically was reported as a change")
	}
	if c, ok := byID["output-moved"]; !ok || c.Worded {
		t.Errorf("a case whose standard output moved must not be filed as a wording change: %+v", c)
	}
	if c, ok := byID["worded-differently"]; !ok || !c.Worded {
		t.Errorf("a case that printed the same output must be filed as a wording change: %+v", c)
	}
	if rep.Behaved() != 1 {
		t.Errorf("Behaved() = %d, want 1", rep.Behaved())
	}
	summary := rep.Summary(true)
	for _, want := range []string{"1/3 unchanged", "what happened changed:", "only the wording changed:"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary does not say %q:\n%s", want, summary)
		}
	}
}

// TestGatedRefusesToRunWithoutAPolicy. A gated column with no policy on it is
// the corpus run twice for nothing, and would report a perfect score for a
// boundary that was never switched on — which is the one failure a mode like
// this must not have.
func TestGatedRefusesToRunWithoutAPolicy(t *testing.T) {
	shell := fakeShell(t)
	missing := filepath.Join(t.TempDir(), "not-here.policy")
	if _, err := RunGated(context.Background(), shell, nil, missing, []Case{{ID: "a", Snippet: "same"}}); err == nil {
		t.Error("a policy that is not there was accepted")
	}
}
