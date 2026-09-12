// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild

import "testing"

// What normalizing must remove is what is true of the machine rather than of
// the shell, and nothing else. It is tested in-package because the two shells
// under comparison usually live in a temporary directory, and the rule that
// hides *those* names would hide the difference this one is for.
func TestNormalizeRemovesOnlyTheMachine(t *testing.T) {
	const shell = "/opt/somewhere/bin/myshell"
	const script = "/usr/bin/thing"

	for _, tc := range []struct{ name, in, want string }{
		{
			// The shell names itself in its diagnostics, and the two shells
			// being compared have different paths.
			"the shell's own path",
			shell + ": line 1: oops",
			"<shell>: line 1: oops",
		},
		{
			"the script's path",
			script + ": line 2: oops",
			"<script>: line 2: oops",
		},
		{
			// A word that merely contains the shell's base name is the
			// script's own output and must survive.
			"a word containing the base name",
			"GNU myshellbug 1.0",
			"GNU myshellbug 1.0",
		},
		{
			// Each run gets a directory whose name differs every time.
			"a temporary directory",
			"wrote /var/folders/ab/cd/T/wild123/f",
			"wrote <tmp>",
		},
		{
			"nothing to remove",
			"plain output",
			"plain output",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalize(tc.in, shell, script); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
