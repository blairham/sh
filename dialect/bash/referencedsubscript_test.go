// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The subscript of an element named by text is expanded before its
// arithmetic reads it. Measured 2026-10-03 under `-c` (#5578). See
// interp.Semantics.ReferencedSubscriptIsExpanded.
func TestAReferencedSubscriptIsExpanded(t *testing.T) {
	src := `a=(x y z); i=1; unset "a[$i]"; echo ${a[@]}; b=(x y z); read "b[$i+1]" <<<R; echo ${b[@]}`
	if out, st := runBash(t, t.TempDir(), src); out != "x z\nx y R\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "x z\nx y R\n")
	}
}
