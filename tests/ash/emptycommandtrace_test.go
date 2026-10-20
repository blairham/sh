// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// **An empty command is traced as the prefix alone** (#5326). Measured
// 2026-10-01 on BusyBox ash 1.37.0 (alpine@sha256:28bd5fe8…) with `sed -n l`:
// `+ ` and a newline for a word that expanded away and for a bare
// redirection; the assignment is the control. See
// interp.Semantics.EmptyCommandTrace.
func TestAnEmptyCommandIsTracedAsThePrefix(t *testing.T) {
	out, _ := runIn(t, `e=; set -x; $e; >/dev/null; v=1 $e; :`)
	if want := "+ \n+ \n+ v=1\n+ :\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
