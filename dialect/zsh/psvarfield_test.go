// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `%v` draws an element of `$psvar` (#5150). Measured 2026-10-01 on zsh 5.9.2
// under `-c`; each row is what that shell wrote.
func TestThePromptArrayElementField(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"psvar=(caesar adsum jam forte); print -P '[%v][%1v][%2v][%4v][%5v][%0v]'", "[caesar][caesar][adsum][forte][][caesar]\n"},
		{"psvar=(caesar adsum jam forte); print -P '[%-v][%-1v][%-2v][%-4v][%-5v]'", "[forte][forte][jam][caesar][]\n"},
		{"psvar=(a '' c); print -P '[%2v][%3v]'", "[][c]\n"},
		{"unset psvar; print -P '[%v]'", "[]\n"},
		{"PSVAR=a:b; print -P '[%v][%2v]'", "[a][b]\n"},
	} {
		if out, st := runZsh(t, dir, c.src+"\n"); out != c.want || st != 0 {
			t.Errorf("%s: got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
