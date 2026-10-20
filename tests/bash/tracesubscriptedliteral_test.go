// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The other side of the axis: a subscripted literal is one line here, the
// words as they were written. Measured 2026-09-17 against bash 5.3.20, a
// script file, `env -i` with LC_ALL=C — and it is the half that must not move
// when the column that writes element assignments is made to write them.
func TestASubscriptedLiteralIsOneLine(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"a=([2]=c [0]=a)", "+ a=([2]=c [0]=a)\n"},
		{"a+=([5]=z)", "+ a+=([5]=z)\n"},
		{"declare -A m\nm=([k]=v [j]=w)", "+ m=([k]=v [j]=w)\n"},
	} {
		out, st := answersRun(t, "set -x\n"+tc.src+"\nset +x\n")
		if st != 0 {
			t.Errorf("%s: status %d, want 0: %q", tc.src, st, out)
			continue
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}
