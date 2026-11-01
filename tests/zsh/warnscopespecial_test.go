// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestScopeLintsPassOverTheShellsOwnParameters pins that neither scope lint
// reports an assignment to a parameter the shell calls `special`, set or
// unset, and that the names beside them which are not special still draw
// their sentence — so a lint gone quiet altogether cannot pass for one that is
// quiet about the shell's own names.
//
// Measured 2026-10-15 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `env -i
// PATH=/usr/bin:/bin TERM=xterm`), each script's whole output (#6425). The
// pairs hold the kind fixed and move only `special`: HOME and LOGNAME are both
// exported scalars, HISTSIZE and MAILCHECK both integers, PS1 and TTY both
// scalars. The integer pair is written as arithmetic, the route both shells
// call `numeric`; the word itself is TestScopeLintWordFollowsTheArithmeticRoute's.
func TestScopeLintsPassOverTheShellsOwnParameters(t *testing.T) {
	t.Parallel()
	const on = "setopt localoptions warn_nested_var warn_create_global"
	for _, tc := range []struct{ name, src, want string }{
		{
			"exported scalar", "f() { " + on + "; HOME=/x; LOGNAME=x; }; f",
			"f: scalar parameter LOGNAME set in enclosing scope in function f\n",
		},
		{
			"integer", "f() { " + on + "; (( HISTSIZE=5 )); (( MAILCHECK=5 )); }; f",
			"f: numeric parameter MAILCHECK set in enclosing scope in function f\n",
		},
		{
			"scalar", "f() { " + on + "; PS1=a; TTY=x; }; f",
			"f: scalar parameter TTY set in enclosing scope in function f\n",
		},
		{
			"special while unset", "f() { " + on + "; LANG=C; LC_ALL=C; TERMINFO=x; EDITOR=x; }; f",
			"f: scalar parameter EDITOR created globally in function f\n",
		},
		{
			"special after an unset", "unset HOME RANDOM; f() { " + on + "; HOME=/x; RANDOM=5; g=1; }; f",
			"f: scalar parameter g created globally in function f\n",
		},
		{
			"tied pair", "f() { " + on + "; path+=(/x); PATH+=:/y; g=1; }; f",
			"f: scalar parameter g created globally in function f\n",
		},
		{
			"a caller's plain local stays special", "f() { local HOME=/a; g; }; g() { " + on + "; HOME=/b; }; f",
			"",
		},
		{
			"a caller's hidden local is ordinary", "f() { local -h HOME=/a; g; }; g() { " + on + "; HOME=/b; }; f",
			"g: scalar parameter HOME set in enclosing scope in function g\n",
		},
		{
			"a caller's hidden local of a name special only while set",
			"f() { local -h LANG=C; g; }; g() { " + on + "; LANG=x; }; f",
			"g: scalar parameter LANG set in enclosing scope in function g\n",
		},
		{
			"one sentence for a script tie's array write",
			"typeset -T FOO foo; f() { " + on + "; FOO=a:b; foo=(c); }; f",
			"f: scalar parameter FOO set in enclosing scope in function f\n" +
				"f: array parameter foo set in enclosing scope in function f\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), "export TERM=xterm; "+tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
