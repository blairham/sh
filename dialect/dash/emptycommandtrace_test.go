// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// **An empty command is traced as the prefix alone** (#5326). Measured
// 2026-10-01 on dash 0.5.12 with `sed -n l`: `+ ` and a newline for a word
// that expanded away and for a bare redirection; the assignment is the
// control. See interp.Semantics.EmptyCommandTrace.
func TestAnEmptyCommandIsTracedAsThePrefix(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `e=; set -x; $e; >/dev/null; v=1 $e; :`)
	if want := "+ \n+ \n+ v=1\n+ :\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
