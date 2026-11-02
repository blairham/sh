// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// dash keeps a special builtin's prefix but opens the redirections before it
// reaches the prefix, so a failed one leaves the name as it was. Measured
// 2026-11-02 from script files under `env -i PATH=/usr/bin:/bin` (#6986): the
// control for ksh93, which stores the prefix first. See
// interp/prefixstoredahead.go.
func TestAPersistingPrefixIsNotStoredPastAFailedRedirection(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"the word reads the old value", `x=0; x=5 : >out_$x; echo out_* "x=$x"`, "out_0 x=5\n"},
		{
			"a failed open leaves it",
			`trap 'echo "x=$x"' EXIT; x=0; x=5 : 2>/dev/null >/nope/x; echo after`, "x=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out, _ := runDash(t, t.TempDir(), c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
