// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestScopeLintsAreSilentAboutTheEnginesOwnState pins that state the shell
// writes on a script's behalf is not an assignment either lint reports. Each
// row's command leaves something in one of the dialect's own stores — the
// terminal set up, a history pushed, a seed advanced — and each row ends with
// an ordinary assignment, which still draws its sentence, so a lint that has
// gone quiet altogether cannot pass for one that is quiet about the store.
//
// Measured 2026-10-12 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -fc`, TERM
// xterm-256color): every row writes exactly the sentence for its last
// assignment and nothing else (#6422).
func TestScopeLintsAreSilentAboutTheEnginesOwnState(t *testing.T) {
	t.Parallel()
	const create = "f: scalar parameter x created globally in function f\n"
	const nested = "f: scalar parameter x set in enclosing scope in function f\n"
	for _, tc := range []struct{ name, src, want string }{
		{
			"print -P", `f() { setopt localoptions warn_create_global; print -P "%B" >/dev/null; x=1; }; f`,
			create,
		},
		{
			"print -P with a color", `f() { setopt localoptions warn_create_global; print -P "%B%F{red}x%f%b" >/dev/null; x=1; }; f`,
			create,
		},
		{
			"prompt expansion flag", `PS1="%B%#"; f() { setopt localoptions warn_create_global; : ${(%)PS1}; x=1; }; f`,
			create,
		},
		{
			"echoti", `f() { setopt localoptions warn_create_global; zmodload zsh/terminfo; echoti bold >/dev/null; x=1; }; f`,
			create,
		},
		{
			"rand48", `f() { setopt localoptions warn_create_global; zmodload zsh/mathfunc; local y; (( y = rand48() )); x=1; }; f`,
			create,
		},
		{
			"fc -p", `f() { setopt localoptions warn_create_global; fc -p; x=1; }; f`,
			create,
		},
		{
			"bindkey -N", `f() { setopt localoptions warn_create_global; bindkey -N mymap; x=1; }; f`,
			create,
		},
		{
			"terminal set up again, nested", `x=0; print -P %B >/dev/null; f() { setopt localoptions warn_nested_var; local TERM=vt100; print -P %B >/dev/null; x=1; }; f`,
			nested,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), "export TERM=xterm-256color; "+tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
