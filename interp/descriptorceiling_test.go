// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A duplication naming a descriptor number past the shell's own ceiling
// (#3210).
//
// The axis is moved both ways over the same numbers, and the rows just below
// the ceiling are the controls: there the ordinary bad-descriptor sentence
// stands whichever answer is held, which is what says the ceiling is a bound
// on the *number* and not a second way of saying a descriptor is not open.
func TestADuplicationPastTheDescriptorCeiling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ceiling DescriptorNumberCeiling
		src     string
		want    string
	}{
		{
			name: "past the ceiling", ceiling: DescriptorNumbersStopAtSixtyFour,
			src: "echo x >&99", want: "99: over the ceiling",
		},
		{
			name: "at the ceiling", ceiling: DescriptorNumbersStopAtSixtyFour,
			src: "echo x >&64", want: "64: over the ceiling",
		},
		{
			name: "just below it", ceiling: DescriptorNumbersStopAtSixtyFour,
			src: "echo x >&63", want: "63: not open",
		},
		{
			name: "with no ceiling", ceiling: NoDescriptorNumberCeiling,
			src: "echo x >&99", want: "99: not open",
		},
		{
			name: "just below it with no ceiling", ceiling: NoDescriptorNumberCeiling,
			src: "echo x >&63", want: "63: not open",
		},
		{
			// The source side of the same operator, which is the other
			// spelling a script writes.
			name: "reading past the ceiling", ceiling: DescriptorNumbersStopAtSixtyFour,
			src: "cat <&99", want: "99: over the ceiling",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.DescriptorNumberCeiling = tc.ceiling
			dg := Diagnostics{
				DuplicationSourceNotOpen: "%[1]s: not open",
				FdNumberOverCeiling:      "%[1]s: over the ceiling",
			}
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &dg
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("out %q, want %q in it", out, tc.want)
			}
		})
	}
	// An empty wording leaves the ordinary sentence standing, which is what
	// every column without a ceiling wants and what the core wants.
	sem := permissive()
	sem.DescriptorNumberCeiling = DescriptorNumbersStopAtSixtyFour
	dg := Diagnostics{DuplicationSourceNotOpen: "%[1]s: not open"}
	out, _ := run(t, "echo x >&99", func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "99: not open") {
		t.Errorf("out %q, want the ordinary sentence where no ceiling wording was given", out)
	}
}
