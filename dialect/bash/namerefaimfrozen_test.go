// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAimingAFrozenReferenceIsRefused — a reference with nothing to point at is
// aimed by its first value, and **aiming is a write to the reference itself**,
// so a frozen one refuses it.
//
// A frozen reference that is already *aimed* is the control on the other side:
// there the value passes through to the target, and the target's own freeze is
// what decides. The plainly written `f=v` already refused this; the declaration
// spelling of the same write went through, so the listing read
// `declare -nr f="v"` where the reference had refused to be aimed (#4178).
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree.
func TestAimingAFrozenReferenceIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"frozen after the letter",
			"typeset -n f\ntypeset -r f\ntypeset f=v\necho \"st=$?\"\ntypeset -p f\n",
			"st=1\ndeclare -nr f\n",
		},
		{
			"frozen with the letter",
			"typeset -nr f\ntypeset f=v\necho \"st=$?\"\ntypeset -p f\n",
			"st=1\ndeclare -nr f\n",
		},
		// The controls. An unfrozen reference is aimed by the same line, and a
		// frozen one that is already aimed writes through to its target.
		{
			"not frozen",
			"typeset -n g\ntypeset g=v\necho \"st=$?\"\ntypeset -p g\n",
			"st=0\ndeclare -n g=\"v\"\n",
		},
		{
			"frozen but already aimed",
			"b=1\ntypeset -nr f=b\ntypeset f=v\necho \"st=$?\"\ntypeset -p b\n",
			"st=0\ndeclare -- b=\"v\"\n",
		},
		// And the plain spelling, which refused it before this and still does
		// — once, and in its own words.
		{
			"written plainly",
			"typeset -nr h\nh=v\necho \"st=$?\"\ntypeset -p h\n",
			"st=1\ndeclare -nr h\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBashSplitFatal(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// The declaration names the builtin and the plain assignment does not,
	// which is the shape every other readonly refusal in this tree takes.
	_, errs := runBashSplitFatal(t, "typeset -nr f\ntypeset f=v\n")
	if want := "typeset: f: readonly variable"; !strings.Contains(errs, want) {
		t.Errorf("stderr = %q, want it to hold %q", errs, want)
	}
	_, errs = runBashSplitFatal(t, "typeset -nr h\nh=v\n")
	if strings.Contains(errs, "typeset:") {
		t.Errorf("stderr = %q, want no builtin named for the plain spelling", errs)
	}
	if n := strings.Count(errs, "readonly variable"); n != 1 {
		t.Errorf("stderr = %q, want the refusal exactly once, got %d", errs, n)
	}
}
