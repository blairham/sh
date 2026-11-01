// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A module name spelled with a character no module name has is refused by
// its spelling before anything is loaded, and the refusal ends the shell
// unless `-s` is written. Measured 2026-10-16 on zsh 5.9.2, `-f`, under `env
// -i PATH=/usr/bin:/bin` (#6787).
func TestZmodloadRefusesAModuleNameBySpelling(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{"zmodload 'zsh/no mod'; echo after", "zsh:1: invalid module name `zsh/no mod'\n", 1},
		{"zmodload 'zsh/a;b' || echo or", "zsh:1: invalid module name `zsh/a;b'\n", 1},
		{"zmodload zsh/a.b", "zsh:1: invalid module name `zsh/a.b'\n", 1},
		{"zmodload zsh/a-b", "zsh:1: invalid module name `zsh/a-b'\n", 1},
		{"zmodload $'zsh/no\\x01mod'", "zsh:1: invalid module name `zsh/no^Amod'\n", 1},
		{"zmodload -F 'zsh/a b' x; echo after", "zsh:1: invalid module name `zsh/a b'\n", 1},
		{"f(){ zmodload 'zsh/a b'; echo in; }; f; echo out", "f: invalid module name `zsh/a b'\n", 1},
		{"(zmodload 'zsh/a b'); echo $?", "zsh:1: invalid module name `zsh/a b'\n1\n", 0},
		{"zmodload -s 'zsh/a b'; echo $?", "1\n", 0},
		// A name spelled well enough reaches the load and its own refusal,
		// and the script carries on.
		{"zmodload -s zsh/a_b/1c; echo $?", "1\n", 0},
		// A bare sign ends the options rather than naming a module.
		{"zmodload - zsh/datetime; echo $? ${+modules[zsh/datetime]}", "0 1\n", 0},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s\n got %q (%d)\nwant %q (%d)", tc.src, out, st, tc.want, tc.status)
		}
	}
}
