// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A listing with no operand answers from this shell's own variables and never
// from the running command's assignment prefix, where the same shell's named
// listing answers from the prefix. Measured 2026-09-18 on bash 5.3.20 from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME
// (#3446).
func TestAWholeTableListingDoesNotSeeTheCommandsPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want, notWant string }{
		{`export k=1; k=9 export -p`, `declare -x k="1"`, `k="9"`},
		{`export k=1; k=9 declare -p`, `declare -x k="1"`, `k="9"`},
		{`export k=1; k=9 set`, "k=1\n", "k=9"},
		// The named listing on the same vector sees the prefix, which is what
		// says this is about the operand-less shape.
		{`export k=1; k=9 declare -p k`, `declare -x k="9"`, `k="1"`},
		// A name the shell holds unexported keeps its own row and is in no
		// export listing, however the prefix's own attribute moved.
		{`m=1; m=9 declare -p`, `declare -- m="1"`, `m="9"`},
		{`m=1; m=9 export -p`, "", "m="},
		// And a name the prefix *created* is in no whole-table listing at
		// all — silently, since nobody asked for it by name.
		{`z=9 export -p`, "", "z="},
		{`z=9 declare -p`, "", "z="},
		{`z=9 set`, "", "z="},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s answered %d: %q", row.src, st, out)
			continue
		}
		if row.want != "" && !strings.Contains(out, row.want) {
			t.Errorf("%s = %q, want it to contain %q", row.src, out, row.want)
		}
		if strings.Contains(out, row.notWant) {
			t.Errorf("%s = %q, want no %q", row.src, out, row.notWant)
		}
	}
}
