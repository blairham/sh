// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// `fg` and `bg` read their options before anything else. Measured 2026-10-04
// on dash 0.5.12 with no job control; the `--` row is the control, where the
// options end and the missing job is what is said.
func TestAResumeRefusesAnOptionFirst(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`bg -x`, "bg: Illegal option -x"},
		{`bg --version`, "bg: Illegal option --"},
		{`fg --version`, "fg: Illegal option --"},
		{`bg --`, "bg: No current job"},
	} {
		out, _ := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}
