// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An empty `ulimit` operand: a limit of nought, or a refusal (#3064).
//
// The axis is moved both ways, and the row that matters is what the limit
// holds *afterwards* — the reading that takes it is not a line that does
// nothing, it sets the limit to zero, so a shell answering the wrong one
// either ends a `set -e` script or closes the script's descriptors.
func TestAnEmptyUlimitOperand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		zero  Answer
		out   string
		st    int
		after int64
	}{
		{
			name: "read as nought", zero: Yes, out: "", st: 0, after: 0,
		},
		{
			name: "refused", zero: No,
			out: "testsh: ulimit: : invalid number\n", st: 1, after: 4096,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held := limits(map[Resource]limitPair{ResourceOpenFiles: {4096, 8192}})
			out, st, after := ulimitRun(t, held,
				func(s *Semantics) { s.UlimitEmptyOperandIsZero = tc.zero },
				`ulimit -n ""`)
			if out != tc.out || st != tc.st {
				t.Errorf("out %q status %d, want %q and %d", out, st, tc.out, tc.st)
			}
			if got := after[ResourceOpenFiles].soft; got != tc.after {
				t.Errorf("the limit is %d afterwards, want %d", got, tc.after)
			}
		})
	}
	// The control: an operand that is a number is read as one whichever way
	// the axis is answered, so the axis reaches nothing but the empty word.
	for _, zero := range []Answer{Yes, No} {
		held := limits(map[Resource]limitPair{ResourceOpenFiles: {4096, 8192}})
		out, st, after := ulimitRun(t, held,
			func(s *Semantics) { s.UlimitEmptyOperandIsZero = zero },
			`ulimit -n 64`)
		if out != "" || st != 0 || after[ResourceOpenFiles].soft != 64 {
			t.Errorf("a written number with the axis %v: out %q status %d limit %d",
				zero, out, st, after[ResourceOpenFiles].soft)
		}
	}
	// And an unanswered axis refuses by name rather than guessing.
	held := limits(map[Resource]limitPair{ResourceOpenFiles: {4096, 8192}})
	out, st, _ := ulimitRun(t, held,
		func(s *Semantics) { s.UlimitEmptyOperandIsZero = Unspecified },
		`ulimit -n ""`)
	if st != 2 || !strings.Contains(out, "the shells disagree here and no dialect was chosen") {
		t.Errorf("unanswered: out %q status %d, want the refusal by name at 2", out, st)
	}
}
