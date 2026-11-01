// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestABuiltinStoreThatRetypesAHiddenNameDropsTheHide pins #6649: a store a
// builtin or an expansion makes into a `local -h` name takes the `hide` word
// off it when it changes the name's kind, as a bare assignment's does, and
// keeps it when the kind stands. Measured 2026-10-15 against zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f` under `env -i PATH=/usr/bin:/bin`, from `-c`
// and from a script file alike).
func TestABuiltinStoreThatRetypesAHiddenNameDropsTheHide(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, want string }{
		{"local -h HOME; set -A HOME x; echo ${(t)HOME}", "array-local"},
		{"local -h v; set -A v x; echo ${(t)v}", "array-local"},
		{"local -h v; set -A v; echo ${(t)v}", "array-local"},
		{"local -h v; set +A v x; echo ${(t)v}", "array-local"},
		{"local -hu v; set -A v x x; echo ${(t)v}", "array-local"},
		{"local -h v; read -A v <<< x; echo ${(t)v}", "array-local"},
		{"local -h v; zparseopts -a v a; echo ${(t)v}", "array-local"},
		{"local -h v; zparseopts -A v a; echo ${(t)v}", "association-local"},
		{"local -h v; : ${(A)v::=x}; echo ${(t)v}", "array-local"},
		{"local -h -a v; read v <<< x; echo ${(t)v}", "scalar-local"},
		{"local -h -A v; read v <<< x; echo ${(t)v}", "scalar-local"},
		{"local -h -a v; printf -v v %s x; echo ${(t)v}", "scalar-local"},
		{"local -h -a v; print -v v x; echo ${(t)v}", "scalar-local"},
		{"local -h -a v; getopts a v -a; echo ${(t)v}", "scalar-local"},
		{"local -h -a v; : ${v::=x}; echo ${(t)v}", "scalar-local"},
		{"local -h -a v; : ${(U)v::=x}; echo ${(t)v}", "scalar-local"},
		{"local -h -a v; for v in x; do :; done; echo ${(t)v}", "scalar-local"},
		// The controls: the kind stands, so the word does.
		{"local -h -a v; set -A v x; echo ${(t)v}", "array-local-hide"},
		{"local -h v=(a b); set +A v x; echo ${(t)v} $v", "array-local-hide x b"},
		{"local -h -A v; zparseopts -A v a; echo ${(t)v}", "association-local-hide"},
		{"local -h v; zparseopts -K -A v a; echo ${(t)v}", "scalar-local-hide"},
		{"local -h -i v; read v <<< x; echo ${(t)v}", "integer-local-hide"},
		{`local -h v; eval "typeset -h -a v"; echo ${(t)v}`, "array-local-hide"},
	} {
		src := "f(){ " + tc.body + "; }; f"
		out, _ := runZshOnPath(t, t.TempDir(), src)
		if out != tc.want+"\n" {
			t.Errorf("%s = %q, want %q", src, out, tc.want+"\n")
		}
	}
}
