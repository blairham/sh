// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// A traced assignment list is written once every assignment has been made, so
// a list that is refused or ends the shell writes no trace line at all.
// Measured 2026-10-02 on BusyBox v1.37.0 in the pinned alpine image under
// `-c` (#5509). The last row is the control: a list that finishes writes its
// line after its substitution's.
func TestATracedAssignmentThatFailsWritesNoLine(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly r; set -x; a=1 r=2`, "ash: r: is read only\n"},
		{`readonly r; set -x; r=1 a=2`, "ash: r: is read only\n"},
		{`set -x; a=1 b=${x?boom}`, "ash: x: boom\n"},
		{`set -ux; a=1 b=$nope c=3`, "ash: nope: parameter not set\n"},
		{`set -x; a=1 b=$((1/0))`, "ash: divide by zero\n"},
		{`set -x; a=1 b=${x?boom} true`, "ash: x: boom\n"},
		{`set -x; a=1 b=$((1/0)) true`, "ash: divide by zero\n"},
		{`set -x; a=1 b=$(echo hi >&2) c=3`, "+ echo hi\nhi\n+ a=1 b= c=3\n"},
	} {
		if out, _ := run(t, c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
