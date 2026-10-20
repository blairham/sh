// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheDirectoryFlag is #5980: `(D)` writes a value as `%~` would draw it,
// under the home as `~` and under a named directory as `~name`, and quotes
// what follows as a single `q` would. Every row measured on zsh 5.9.2
// (/opt/homebrew/bin/zsh, -f), 2026-10-05, with the same `HOME` and table.
func TestTheDirectoryFlag(t *testing.T) {
	const pre = `HOME=/Users/x; hash -d proj=/opt/p; hash -d pp=/opt/p/src
a=/Users/x/doc; v='/Users/x/a b'; arr=(/Users/x /opt/p/src/q /Users/xy)
`
	for _, tc := range []struct{ src, want string }{
		{`print -r -- ${(D)a} ${(D)${:-/opt/p}} ${(D)${:-/tmp}}`, `~/doc ~proj /tmp`},
		// The shortest drawing wins, and a near miss is no directory.
		{`print -r -- ${(D)arr}`, `~ ~pp/q /Users/xy`},
		{`print -r -- "${(D)arr}"`, `/Users/x\ /opt/p/src/q\ /Users/xy`},
		{`print -rl -- "${(@D)arr}"`, "~\n~pp/q\n/Users/xy"},
		// Quoted whether or not anything was abbreviated, the drawn tilde
		// word excepted and a written one included.
		{`print -r -- ${(D)v} "${(D)v}"`, `~/a\ b ~/a\ b`},
		{`print -r -- ${(D)${:-"/q/it's"}} ${(D)${:-"*"}}`, `/q/it\'s \*`},
		{`x='~/q'; print -r -- ${(D)x}`, `\~/q`},
		{`e=; print -r -- "[${(D)e}]"`, `[]`},
		{`u=$'/Users/x/a\nb'; print -r -- ${(D)u}`, `~/a$'\n'b`},
		// A trailing separator abbreviates nothing, nor does a home of `/`.
		{`print -r -- ${(D)${:-/Users/x/}}`, `~/`},
		{`HOME=/; print -r -- ${(D)${:-/etc}}`, `/etc`},
		// Last of the value's rewrites: after the case, the operator, the
		// quoting and the unquoting, and ahead of `V`.
		{`print -r -- ${(UD)a} ${(D)a:h} ${(D)a#/Users} ${(D)a[1,9]}`, `/USERS/X/DOC ~ /x/doc ~/`},
		{`print -r -- ${(qqD)v}`, `\'/Users/x/a\ b\'`},
		{`w='/Users/x/a\b'; print -r -- ${(DQ)w}`, `~/ab`},
		{`u=$'/Users/x/a\tb'; print -r -- ${(DV)u}`, `~/a$'\t'b`},
		{`print -r -- ${(D)#a}`, `12`},
	} {
		out, st := runZsh(t, t.TempDir(), pre+tc.src)
		if out != tc.want+"\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// TestVisibleRunsAfterTheUnquoting: with `t=$'a\tb'`, zsh 5.9.2 draws
// `${(VQ)t}` as `a\tb`. A `V` ahead of the `Q` hands it a backslash to take,
// and gives `atb`.
func TestVisibleRunsAfterTheUnquoting(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `t=$'a\tb'; print -r -- ${(VQ)t} ${(QV)t}`)
	if want := `a\tb a\tb` + "\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
