// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **An empty command is traced as a bare line** (#5326). Measured 2026-10-01
// on zsh 5.9.2 under `-f -c` with `sed -n l`: a newline with no prefix, for a
// word that expanded away, for one inside a brace group and for a line of
// modifiers with nothing behind them; the assignment is the control, which
// is the bare assignment's own line. See interp.Semantics.EmptyCommandTrace.
func TestAnEmptyCommandIsTracedAsABareLine(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `e=; set -x; $e; { $e; }; builtin; v=1 $e; :`)
	if want := "\n\n\n+zsh:1> v=1 \n+zsh:1> :\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
