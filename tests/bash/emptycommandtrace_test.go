// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// **An empty command is not traced at all** (#5326). Measured 2026-10-01 on
// bash 5.3.20 and ksh93u+ with `sed -n l`: nothing for a word that expanded
// away; the assignment is the control. See interp.Semantics.EmptyCommandTrace.
func TestAnEmptyCommandIsNotTraced(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `e=; set -x; $e; v=1 $e; :`)
	if want := "+ v=1\n+ :\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
