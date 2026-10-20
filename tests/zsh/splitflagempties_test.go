// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An unquoted IFS split in a flag group keeps the empty fields two adjacent
// non-whitespace separators make, as the flag-free `${=u}` does (#5275). An
// `(s)` split and an unquoted array's empty elements still drop theirs.
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`); each row is what that shell wrote.
func TestAnUnquotedIFSSplitInAGroupKeepsItsEmptyFields(t *testing.T) {
	dir := t.TempDir()
	const setup = "show() { for x in \"$@\"; do printf '<%s>' \"$x\"; done; }\nIFS=:; u='a::b:'; a=(x '' y)\n"
	for _, c := range []struct{ name, src, want string }{
		{"(@)=", `show ${(@)=u}`, "<a><><b><>"},
		{"(U)=", `show ${(U)=u}`, "<A><><B><>"},
		{"(@)= with an operator", `show ${(@)=u:-x}`, "<a><><b><>"},
		{"(@)= over an array, joined first", `show ${(@)=a}`, "<x><><y>"},
		// The controls: the flag-free split, which was already right, and
		// the two readings that really do drop their empties.
		{"= with no group", `show ${=u}`, "<a><><b><>"},
		{"(s) drops them", `show ${(@s.:.)u}`, "<a><b>"},
		{"an array's empty element goes", `show ${(@)a}`, "<x><y>"},
		{"an empty value is no field", `v=''; show ${(@)=v}`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, setup+c.src+"\n"); out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
		})
	}
}
