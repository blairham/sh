// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A traced assignment list is written once every assignment has been made, so
// a list that is refused or ends the shell writes no trace line at all — and
// so does a prefixed command whose prefix holds a frozen name, though its
// value is still expanded. Measured 2026-10-02 on dash 0.5.12 under `-c` with
// `sed -n l` (#5509). The last row is the control: a list that finishes
// writes its line after its substitution's.
func TestATracedAssignmentThatFailsWritesNoLine(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly r; set -x; a=1 r=2`, "dash: 1: r: is read only\n"},
		{`readonly r; set -x; r=1 a=2`, "dash: 1: r: is read only\n"},
		{`set -x; a=1 b=${x?boom}`, "dash: 1: x: boom\n"},
		{`set -ux; a=1 b=$nope c=3`, "dash: 1: nope: parameter not set\n"},
		{`set -x; a=1 b=$((1/0))`, "dash: 1: arithmetic expression: division by zero: \"1/0\"\n"},
		{`set -x; a=1 b=${x?boom} true`, "dash: 1: x: boom\n"},
		{`readonly r; set -x; a=1 r=2 true; echo st=$?`, "dash: 1: r: is read only\n"},
		{`readonly x; set -x; x=$(echo s >&2) true`, "+ echo s\ns\ndash: 1: x: is read only\n"},
		{`set -x; a=1 b=$(echo hi >&2) c=3`, "+ echo hi\nhi\n+ a=1 b= c=3\n"},
	} {
		if out, _ := runDash(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
