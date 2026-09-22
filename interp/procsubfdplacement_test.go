// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Where a process substitution's far end lands in the descriptor table — the
// N of the `/dev/fd/N` the word expands to.
//
// Four shells have the construct and each numbers it differently. Measured
// 2026-09-21 with `echo <(true) <(true) <(true)`, ash through the pinned
// BusyBox image: bash 5.3.20 and 3.2.57 `63 62 61`, zsh 5.9.2 `11 12 13`,
// ksh93 `3 4 5`, BusyBox 1.37.0 `64 65 66`. This shell answered `10 11 12` in
// every dialect. See Semantics.SubstitutionEndPlacement.
//
// # What a test in this process can and cannot hold
//
// These numbers come from the kernel's table for the **whole test binary**,
// so a parallel test that opens a file moves them: this case was first
// written asserting the three digits and answered `63 58 62` and `13 15 14`
// under `go test ./interp/`, which is the rule working beside other tests
// rather than the rule being wrong. So what is asserted here is the
// **region** each rule allocates from, which no other test can perturb —
// upward rules never answer below their base, and the downward rule never
// answers above 63.
//
// The digits themselves are pinned where the table is quiet and the shell is
// the only thing in the process: against the real shells through each built
// dialect binary, which is where the four rows above were measured, and by
// the bash column of `make bash-suite`, which runs a whole file of them.
func TestASubstitutionEndTakesTheDialectsRegion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		where   SubstEndPlacement
		base    DescriptorAllocationBase
		lowest  int
		highest int
	}{
		{
			"the top of the table, at or below 63", SubstitutionEndsAtTheTopOfTheTable,
			AllocateDescriptorsFromTen, 3, 63,
		},
		{
			"above the top of the table, at or above 64", SubstitutionEndsAboveTheTopOfTheTable,
			AllocateDescriptorsFromTen, 64, 1 << 20,
		},
		{
			"where any descriptor goes, at or above the base of 11",
			SubstitutionEndsWhereAnyDescriptorGoes, AllocateDescriptorsFromEleven, 11, 1 << 20,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := substNumbers(t, runSubstPlacement(t, tc.where, tc.base, nil,
				"echo <(true) <(true) <(true)"))
			if len(got) != 3 {
				t.Fatalf("got %v, want three numbers", got)
			}
			for _, n := range got {
				if n < tc.lowest || n > tc.highest {
					t.Errorf("got %v, want every number in [%d,%d]", got, tc.lowest, tc.highest)
				}
			}
		})
	}
}

// A number the script parked before the word expanded is stepped over.
//
// Measured the same day on bash 5.3.20: `exec 63</dev/null; echo <(true)
// <(true)` is `/dev/fd/62 /dev/fd/61`, so the descent skips what is held
// rather than colliding with it or stopping at it. The collision this rules
// out is the one firstProcSubFd was originally set to 10 to avoid — two
// entries on one number in the table childFiles builds, with which of them
// survived decided by map iteration order.
func TestASubstitutionEndStepsOverAParkedNumber(t *testing.T) {
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil,
		"exec 63</dev/null\necho <(true) <(true)"))
	for _, n := range got {
		if n == 63 {
			t.Errorf("got %v, want the parked 63 stepped over", got)
		}
	}
}

// The top of the table gives way to the open-file limit.
//
// Measured 2026-09-21 by sweeping `ulimit -n`, two substitutions in one
// command: bash is `63 62` at every limit of 64 and above and `3 4` at 63 and
// below — so the test is whether 63 is a legal descriptor, the same constant
// and the same condition Semantics.CoprocessEndPlacement records. What it
// gives way *to* is the lowest free number, which is ksh93's rule, so this
// axis's second value is what the third one degrades into rather than an
// invention.
//
// The assertion is one-sided for the reason the first case in this file
// gives: below the limit the number is free to be anything the process had
// spare, and only "never at or above the limit it was told about" is a
// property of the rule rather than of the table.
func TestTheSubstitutionTopOfTheTableGivesWayToTheOpenFileLimit(t *testing.T) {
	for _, soft := range []int64{63, 64, 1024, RlimitInfinity} {
		t.Run(strconv.FormatInt(soft, 10), func(t *testing.T) {
			limit := func(Resource) (int64, int64, error) { return soft, soft, nil }
			got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
				AllocateDescriptorsFromTen, limit, "echo <(true)"))
			if len(got) != 1 {
				t.Fatalf("got %v, want one number", got)
			}
			if soft != RlimitInfinity && int64(got[0]) >= soft {
				t.Errorf("got %v, want a number below the limit of %d", got, soft)
			}
			if got[0] > 63 {
				t.Errorf("got %v, want a number at or below the top of the table", got)
			}
		})
	}
}

// A wish the kernel refuses is a wish that missed, not the end of the search.
//
// This is the half a list of preferred numbers gets wrong by default. Asking
// for 63 under a low `ulimit -n` is EMFILE or EINVAL rather than a number,
// and a search that reported that error would turn a construct every shell
// answers into a diagnostic: measured while writing this, `ulimit -n 64` and
// a substitution was `too many open files` at 1 where bash prints a path and
// carries on.
func TestARefusedSubstitutionNumberIsNotTheEndOfTheSearch(t *testing.T) {
	limit := func(Resource) (int64, int64, error) { return 64, 64, nil }
	out := runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, limit, "echo <(true)")
	if !strings.HasPrefix(out, "/dev/fd/") {
		t.Errorf("got %q, want a substitution path", out)
	}
}

// substNumbers is the descriptor numbers of a run's substitution paths.
func substNumbers(t *testing.T, out string) []int {
	t.Helper()
	var ns []int
	for _, f := range strings.Fields(strings.TrimSpace(out)) {
		n, err := strconv.Atoi(strings.TrimPrefix(f, "/dev/fd/"))
		if err != nil {
			t.Fatalf("not a substitution path: %q", f)
		}
		ns = append(ns, n)
	}
	return ns
}

// runSubstPlacement runs src with process substitution on, under the given
// placement and allocation base, optionally under an embedder-supplied
// open-file limit.
//
// Multi-digit descriptor numbers are on because one case writes `exec
// 63</dev/null`, which is the only way for a script to reach a number the
// shell would otherwise pick for itself.
func runSubstPlacement(t *testing.T, where SubstEndPlacement,
	base DescriptorAllocationBase, limit func(Resource) (int64, int64, error), src string,
) string {
	t.Helper()
	d := syntax.Core()
	d.ProcessSubstitution = true
	d.FdVariableRedirections = true
	d.MultiDigitFdNumber = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.SubstitutionEndPlacement = where
	sem.FirstAllocatedDescriptor = base
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: &strings.Builder{}, GetRlimit: limit,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
