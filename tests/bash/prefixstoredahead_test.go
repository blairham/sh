// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash expands a prefix ahead of the redirections but stores it behind them,
// so in POSIX mode, where `:` keeps its prefix, a failed redirection leaves
// the name as it was and the redirection's word reads the old value. Measured
// 2026-11-02 on 5.3.20 from script files under `env -i PATH=/usr/bin:/bin`
// (#6986): the control for ksh93, which stores it first. See
// interp/prefixstoredahead.go.
func TestAPersistingPrefixIsStoredBehindTheRedirections(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"the word reads the old value", `set -o posix; x=0; x=5 : >out_$x; echo out_* "x=$x"`, "out_0 x=5\n"},
		{
			"a failed open leaves it",
			`set -o posix; trap 'echo "x=$x"' EXIT; x=0; x=5 : 2>/dev/null >/nope/x; echo after`, "x=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out, _ := runBash(t, t.TempDir(), c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
