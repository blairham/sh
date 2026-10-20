// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// This shell works through a command's assignment prefix before it opens the
// redirections only where the assignment is a real store — a function or a
// special builtin — which is exactly the set whose prefix it keeps. A regular
// builtin and an external open the redirections first, so a substitution in a
// value never runs when one fails. Measured 2026-09-18 on ksh93u+ 2012-08-01
// from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` (#3449).
func TestThePrefixIsExpandedFirstOnlyWhereItPersists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct {
		src   string
		first bool
	}{
		{`f() { :; }; w=$(echo S >&2) f > /nope/x`, true},
		{`w=$(echo S >&2) eval : > /nope/x`, true},
		{`w=$(echo S >&2) true > /nope/x`, false},
		{`w=$(echo S >&2) print hi > /nope/x`, false},
		{`w=$(echo S >&2) /usr/bin/true > /nope/x`, false},
	} {
		out, st := runKsh(t, dir, row.src)
		if got := strings.HasPrefix(out, "S\n"); got != row.first || st == 0 {
			t.Errorf("%s = %q (status %d), expanded first = %v, want %v",
				row.src, out, st, got, row.first)
		}
	}
}
