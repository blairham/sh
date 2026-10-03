// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestUnsetKeepsTheShellsOwnAttributes pins that `unset` on one of the shell's
// own parameters takes its value and leaves its export and type letters, so
// an assignment brings it back as it was. For a name a script made, the
// letters go with the value. Measured 2026-10-03 on zsh 5.9.2 under
// `env -i PATH=/usr/bin:/bin`. See Semantics.UnsetKeepsTheShellsOwnAttributes.
func TestUnsetKeepsTheShellsOwnAttributes(t *testing.T) {
	// What a child is handed of the names in pat, read without any program
	// but env.
	env := func(pat string) string {
		return "for kv in ${(f)\"$(/usr/bin/env)\"}; do [[ $kv == (" + pat + ")=* ]] && print -r -- $kv; done\n"
	}
	for _, tc := range []struct{ src, want string }{
		{"export SHLVL=5; unset SHLVL; SHLVL=1\n" + env("SHLVL"), "SHLVL=1\n"},
		{"export SHLVL=5; unset SHLVL; unset SHLVL; SHLVL=3\n" + env("SHLVL"), "SHLVL=3\n"},
		{"export HOME=/h FOO=1; unset HOME FOO; HOME=/g FOO=2\n" + env("HOME|FOO"), "HOME=/g\n"},
		{"export PS1=x; unset PS1; PS1=y; print ${(t)PS1}\n" + env("PS1"), "scalar-export-special\nPS1=y\n"},
		// While it is unset nothing of it shows.
		{"export SHLVL=5; unset SHLVL; print ${+SHLVL} ${(t)SHLVL}.\n" + env("SHLVL"), "0 .\n"},
		// The type letters stay too.
		{`typeset -x HISTSIZE; unset HISTSIZE; HISTSIZE=3+4; print $HISTSIZE ${(t)HISTSIZE}`, "7 integer-export-special\n"},
		{`export SECONDS=5; unset SECONDS; SECONDS=7; print ${(t)SECONDS}`, "integer-export-special\n"},
		// What is kept is what it had: an unexported one is not exported.
		{"export SHLVL=5; typeset +x SHLVL; unset SHLVL; SHLVL=3\n" + env("SHLVL"), ""},
		// The controls: a name a script made loses both letters.
		{"typeset -ix n=5; unset n; n=3+4; print $n ${(t)n}\n", "3+4 scalar\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
