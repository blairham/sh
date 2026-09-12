// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The builtin corners: option listings, the silent integer refusal, the
// bare read, and fc — each named by axis or diagnostic value.

func TestSetOListsInTheDialectsColumns(t *testing.T) {
	out, st := run(t, `set -e; set -o`, func(r *Runner) {
		dg := Diagnostics{OptionListingHeader: "Current option settings", OptionListingWidth: 16}
		r.Diagnostics = &dg
	})
	if st != 0 || !strings.HasPrefix(out, "Current option settings\n") {
		t.Errorf("out=%q st=%d, want the header first", out, st)
	}
	if !strings.Contains(out, "errexit         on") {
		t.Errorf("out=%q, want errexit padded to sixteen and on", out)
	}
	out, _ = run(t, `set +o`, nil)
	if !strings.Contains(out, "set +o errexit") {
		t.Errorf("out=%q, want re-inputtable lines", out)
	}
	out, _ = run(t, `set -e; set +o`, func(r *Runner) {
		dg := Diagnostics{PlusOListsActive: true}
		r.Diagnostics = &dg
	})
	if !strings.HasPrefix(out, "set --default") || !strings.Contains(out, "--errexit") {
		t.Errorf("out=%q, want the one-line active form", out)
	}
}

func TestKillListingShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		form KillListingForm
		want func(string) bool
	}{
		{"numbered", KillListingNumbered, func(s string) bool {
			return strings.HasPrefix(s, " 1) SIGHUP\t 2) SIGINT")
		}},
		{"space joined", KillListingSpaceJoined, func(s string) bool {
			first := strings.SplitN(s, "\n", 2)[0]
			return strings.HasPrefix(first, "HUP INT ")
		}},
		{"zero first", KillListingZeroFirst, func(s string) bool {
			return strings.HasPrefix(s, "0\nHUP\n")
		}},
		{"per line", KillListingPerLine, func(s string) bool {
			return strings.HasPrefix(s, "HUP\nINT\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `kill -l`, func(r *Runner) {
				dg := Diagnostics{KillListing: tc.form}
				r.Diagnostics = &dg
			})
			if !tc.want(out) {
				t.Errorf("out=%q, wrong shape", out)
			}
		})
	}
}

// The silence a non-numeric operand can meet is no longer an axis of its own:
// where a dialect reads the operands as arithmetic, `a` is an unset name and
// so zero, which is unequal to one and says nothing about it. That reading is
// TestBuiltinComparisonOperandsAreArithmetic and the two sides of it are in
// testcomparison_test.go; what is kept here is the answer the axis it replaced
// was written for.
func TestIntegerRefusalCanBeSilent(t *testing.T) {
	out, st := run(t, `[ a -eq 1 ]`, func(r *Runner) {
		sem := CoreSemantics()
		sem.TestBuiltinComparisonOperandsAreArithmetic = Yes
		r.Semantics = &sem
	})
	if st != 1 || out != "" {
		t.Errorf("out=%q st=%d, want a plain silent false", out, st)
	}
}

func TestABareReadCanRequireAName(t *testing.T) {
	out, st := run(t, `read`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ReadRequiresAVariableName = Yes
		r.Semantics = &sem
		dg := Diagnostics{ReadArgCount: "read: arg count"}
		r.Diagnostics = &dg
	})
	if st != 2 || !strings.Contains(out, "read: arg count") {
		t.Errorf("out=%q st=%d, want the refusal at 2", out, st)
	}
}

func TestFcAnswersFromItsEmptyHistory(t *testing.T) {
	out, st := run(t, `fc -l`, nil)
	if st != 0 || out != "" {
		t.Errorf("out=%q st=%d, want silence at 0", out, st)
	}
	out, st = run(t, `fc -l`, func(r *Runner) {
		sem := CoreSemantics()
		sem.FcEmptyHistoryIsAnError = Yes
		r.Semantics = &sem
		dg := Diagnostics{FcNoSuchEvent: "fc: no such event: 1"}
		r.Diagnostics = &dg
	})
	if st != 1 || !strings.Contains(out, "no such event") {
		t.Errorf("out=%q st=%d, want the event reported at 1", out, st)
	}
}
