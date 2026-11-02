// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zsh opens the redirections before it reaches the prefix, so a failed one
// leaves the name as it was; the complaint is the shell's, so a group takes
// it. Measured 2026-11-02 on 5.9.2 from script files
// under `env -i PATH=/usr/bin:/bin` (#6986): the control for ksh93, which
// stores the prefix first. See interp/prefixstoredahead.go.
func TestAPrefixIsNotStoredPastAFailedRedirection(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a function", `f() { :; }; x=0; { x=5 f >/nope/x; } 2>/dev/null; echo "x=$x"`, "x=0\n"},
		{"a special builtin", `x=0; { x=5 : >/nope/x; } 2>/dev/null; echo "x=$x"`, "x=0\n"},
		{"the word reads the old value", `x=0; x=5 : >out_$x; echo out_*`, "out_0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out, _ := runZsh(t, t.TempDir(), c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
