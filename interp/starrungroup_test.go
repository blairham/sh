// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A run of stars collapses to one, but the run stops at a star that opens a
// group.
//
// `**(e|f)` is a wildcard followed by the closure `*(e|f)`, and the dialect
// with quantified groups has no bare ones — so taking both stars left `(e|f)`
// to be matched as five ordinary characters, and `ab**(e|f)` reached none of
// the names `ab*(e|f)` does.
//
// Measured 2026-09-22 on bash 5.3.20 with `extglob` and on ksh93u+, in a
// directory holding `ab`, `abef`, `abcdef` and `abcfef`. The five rows are
// each other's controls: they differ only in what stands between the `ab` and
// the group, and each quantifier answers differently.
func TestAStarRunStopsAtAStarThatOpensAGroup(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"ab", "abef", "abcdef", "abcfef"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ pattern, want string }{
		{`ab*(e|f)`, `[ab][abef]`},
		{`ab**(e|f)`, `[ab][abcdef][abcfef][abef]`},
		{`ab***(e|f)`, `[ab][abcdef][abcfef][abef]`},
		{`ab*+(e|f)`, `[abcdef][abcfef][abef]`},
		{`ab*@(e|f)`, `[abcdef][abcfef][abef]`},
		{`ab?*(e|f)`, `[abcfef][abef]`},
		// The star run with nothing behind it is unchanged, which is what
		// says the stop is the group's doing and not the run's.
		{`ab**`, `[ab][abcdef][abcfef][abef]`},
		{`**`, `[ab][abcdef][abcfef][abef]`},
	} {
		if got := runExtendedIn(t, dir, `printf "[%s]" `+tc.pattern); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}
